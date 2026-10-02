package services

import (
	"encoding/json"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"machdown/server/models"
	"machdown/server/repositories"
	"machdown/server/storage"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const chunkSize = 4 * 1024 * 1024 // 4MB per chunk

// ProgressEvent é o evento enviado via SSE para os clientes conectados.
type ProgressEvent struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	BytesDone int64  `json:"bytes_done"`
	TotalSize int64  `json:"total_size"`
	// TargetClients is the job's JSON array of client IDs; used only to route events.
	TargetClients string `json:"-"`
}

// IsFor reports whether the event should reach clientID. An empty clientID
// means the subscriber did not identify itself and receives everything.
func (e ProgressEvent) IsFor(clientID string) bool {
	if clientID == "" {
		return true
	}
	var targets []string
	if err := json.Unmarshal([]byte(e.TargetClients), &targets); err != nil {
		return false
	}
	for _, t := range targets {
		if t == clientID {
			return true
		}
	}
	return false
}

// ProgressHub gerencia os canais SSE dos clientes conectados.
type ProgressHub struct {
	mu      sync.RWMutex
	clients map[chan ProgressEvent]struct{}
}

func NewProgressHub() *ProgressHub {
	return &ProgressHub{
		clients: make(map[chan ProgressEvent]struct{}),
	}
}

// Subscribe registra um novo canal de cliente SSE.
func (h *ProgressHub) Subscribe() chan ProgressEvent {
	ch := make(chan ProgressEvent, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe remove e fecha o canal do cliente SSE.
func (h *ProgressHub) Unsubscribe(ch chan ProgressEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// Broadcast envia um evento para todos os clientes SSE conectados (non-blocking).
func (h *ProgressHub) Broadcast(evt ProgressEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- evt:
		default:
			// cliente lento: descarta para não bloquear
		}
	}
}

type DownloadService struct {
	repo       *repositories.JobRepository
	downloader *ChunkDownloader
	disk       *storage.DiskManager
	Hub        *ProgressHub
	resolver   *LinkResolver

	activeJobs map[string]*models.DownloadJob
	activeMu   sync.RWMutex
	activeDownloads chan struct{}
}

func NewDownloadService(repo *repositories.JobRepository, cd *ChunkDownloader, disk *storage.DiskManager, hub *ProgressHub, maxConcurrent int) *DownloadService {
	if maxConcurrent <= 0 {
		maxConcurrent = 8
	}
	return &DownloadService{
		repo:            repo,
		downloader:      cd,
		disk:            disk,
		Hub:             hub,
		resolver:        NewLinkResolver(),
		activeJobs:      make(map[string]*models.DownloadJob),
		activeDownloads: make(chan struct{}, maxConcurrent),
	}
}

func (s *DownloadService) GetCompletedJobs(clientID string) ([]models.DownloadJob, error) {
	return s.repo.FindCompletedJobsByClient(clientID)
}

func (s *DownloadService) MarkJobSynced(jobID string) error {
	return s.repo.UpdateJobStatus(jobID, "Synced")
}

// GetFilePath expõe o path físico do arquivo para o controller servir via HTTP.
func (s *DownloadService) GetFilePath(jobID string) string {
	job, err := s.repo.FindJobByID(jobID)
	if err != nil || job == nil {
		return ""
	}
	return s.disk.FindFilePath(jobID)
}

func getCategoryFromFileName(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv":
		return "Videos"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg":
		return "Audio"
	case ".exe", ".msi", ".dmg", ".pkg", ".deb", ".rpm", ".appimage":
		return "Programs"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt":
		return "Documents"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "Archives"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg":
		return "Images"
	default:
		return "Others"
	}
}


func (s *DownloadService) Enqueue(rawURL string, targetClients string, cookies string, userAgent string) (*models.DownloadJob, error) {
	if err := isInternalURL(rawURL); err != nil {
		return nil, fmt.Errorf("SSRF guard: invalid or internal URL: %w", err)
	}

	if existingJob, err := s.repo.FindByURL(rawURL); err == nil && existingJob != nil {
		if existingJob.Status != "Failed" && existingJob.Status != "Error" {
			return nil, fmt.Errorf("URL already exists or is downloading")
		}
	}

	// --- Resolução do link direto de download ---
	// Muitos sites de file hosting (WorkUpload, Mediafire, etc.) não entregam
	// o arquivo diretamente pela URL da página — precisamos resolver o link real.
	// ResolveWithSession retorna também os cookies de sessão obtidos (ex: WorkUpload PoW).
	resolved := s.resolver.ResolveWithSession(rawURL, cookies, userAgent)
	url := resolved.URL
	if resolved.Cookies != "" && resolved.Cookies != cookies {
		log.Printf("[ENQUEUE] Usando cookies de sessão do resolver para %s", url)
		cookies = resolved.Cookies
	}
	if url != rawURL {
		log.Printf("[ENQUEUE] URL resolvida: %s → %s", rawURL, url)
	}

	// --- Deduplicação via HEAD request ---
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	client := GetSharedHTTPClient()
	client.CheckRedirect = SafeCheckRedirect(10)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Verificar ETag para deduplicação real
	etag := resp.Header.Get("ETag")
	if etag != "" {
		existing, lookupErr := s.repo.FindJobByETag(etag)
		if lookupErr == nil && existing != nil {
			log.Printf("[DEDUP] Job para ETag %s já existe (ID: %s). Retornando job existente.", etag, existing.ID)
			return existing, nil
		}
	}

	// Parse Content-Length
	lengthStr := resp.Header.Get("Content-Length")
	var totalSize int64
	if lengthStr != "" {
		totalSize, _ = strconv.ParseInt(lengthStr, 10, 64)
	}

	// Parse filename
	fileName := "downloaded_file"
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		_, params, parseErr := mime.ParseMediaType(cd)
		if parseErr == nil && params["filename"] != "" {
			fileName = params["filename"]
		}
	}
	if fileName == "downloaded_file" {
		base := filepath.Base(url)
		if base != "" && base != "/" && base != "." {
			fileName = base
		}
	}

	job := &models.DownloadJob{
		ID:            uuid.New().String(),
		URL:           url,
		FileName:      fileName,
		Category:      getCategoryFromFileName(fileName),
		TotalSize:     totalSize,
		ETag:          etag,
		Status:        "Queued",
		TargetClients: targetClients,
		Cookies:       cookies,
		UserAgent:     userAgent,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = s.repo.CreateJob(job)
	if err != nil {
		return nil, err
	}

	log.Printf("[ENQUEUE] Job %s criado para URL: %s (Size: %d bytes, ETag: %s)", job.ID, job.URL, totalSize, etag)
	go s.processJob(job)

	return job, nil
}

func (s *DownloadService) processJob(job *models.DownloadJob) {
	s.activeDownloads <- struct{}{}
	defer func() { <-s.activeDownloads }()

	log.Printf("[JOB %s] Iniciando processamento.", job.ID)
	_ = s.repo.UpdateJobStatus(job.ID, "Downloading")
	job.Status = "Downloading"
	s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Downloading", TotalSize: job.TotalSize})

	ctx, cancel := context.WithCancel(context.Background())
	job.CancelFunc = cancel

	s.activeMu.Lock()
	s.activeJobs[job.ID] = job
	s.activeMu.Unlock()
	defer func() {
		s.activeMu.Lock()
		delete(s.activeJobs, job.ID)
		s.activeMu.Unlock()
	}()

	if job.TotalSize == 0 {
		log.Printf("[JOB %s] Tamanho desconhecido (sem Content-Length). Usando download sequencial.", job.ID)
		err := s.downloader.DownloadSequential(ctx, job)
		if err != nil {
			log.Printf("[JOB %s] Download sequencial falhou: %v", job.ID, err)
			_ = s.repo.UpdateJobStatus(job.ID, "Error")
			s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Error"})
		} else {
			log.Printf("[JOB %s] Download sequencial concluído.", job.ID)
			
			filePath := s.disk.FindFilePath(job.ID)
			fileHash, hErr := computeFileSHA256(filePath)
			if hErr == nil && fileHash != "" {
				job.RootHash = fileHash
				_ = s.repo.UpdateRootHash(job.ID, fileHash)
				log.Printf("[JOB %s] Hash SHA-256 sequencial calculado: %s", job.ID, fileHash)
			}
			
			// O nome do arquivo pode ter sido descoberto via Content-Disposition durante o download
			_ = s.repo.UpdateJobFileName(job.ID, job.FileName)
			
			_ = s.repo.UpdateJobStatus(job.ID, "Completed")
			s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Completed", BytesDone: job.TotalSize, TotalSize: job.TotalSize})
		}
		return
	}

	// Alocar arquivo esparso com categoria correta
	_, err := s.disk.AllocateFile(job.ID, job.Category, job.TotalSize)
	if err != nil {
		log.Printf("[JOB %s] Erro ao alocar arquivo esparso: %v", job.ID, err)
		_ = s.repo.UpdateJobStatus(job.ID, "Error")
		s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Error"})
		return
	}

	s.startChunks(ctx, job, false)
}

func computeFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func cleanupChunkFiles(filePath string) {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	matches, err := filepath.Glob(filepath.Join(dir, base+".chunk_*"))
	if err == nil {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}
}

func (s *DownloadService) startChunks(ctx context.Context, job *models.DownloadJob, isResume bool) {
	// Dividir em chunks
	var allChunks []models.ChunkTask
	for i := int64(0); i < job.TotalSize; i += chunkSize {
		end := i + chunkSize - 1
		if end >= job.TotalSize {
			end = job.TotalSize - 1
		}
		allChunks = append(allChunks, models.ChunkTask{
			ID:        uuid.New().String(),
			JobID:     job.ID,
			StartByte: i,
			EndByte:   end,
			Status:    "Pending",
		})
	}

	filePath := s.disk.FindFilePath(job.ID)
	var pendingChunks []models.ChunkTask
	bytesDone := int64(0)
	for _, c := range allChunks {
		chunkFile := fmt.Sprintf("%s.chunk_%d", filePath, c.StartByte)
		if _, err := os.Stat(chunkFile); os.IsNotExist(err) {
			pendingChunks = append(pendingChunks, c)
		} else {
			bytesDone += c.EndByte - c.StartByte + 1
		}
	}
	job.BytesDownloaded = bytesDone

	log.Printf("[JOB %s] Arquivo alocado. Total: %d chunks (%d pendentes). Iniciando workers...", job.ID, len(allChunks), len(pendingChunks))

	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		failedCount int
		firstErr    error
	)
	sem := make(chan struct{}, 8)

	for _, c := range pendingChunks {
		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(chunk models.ChunkTask) {
			defer wg.Done()
			defer func() { <-sem }() // release

			err := s.downloader.DownloadChunk(ctx, job, &chunk)
			if err != nil {
				log.Printf("[JOB %s] Chunk %d-%d falhou: %v", job.ID, chunk.StartByte, chunk.EndByte, err)
				mu.Lock()
				failedCount++
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			} else {
				log.Printf("[JOB %s] Chunk %d-%d concluído com hash: %s", job.ID, chunk.StartByte, chunk.EndByte, chunk.Hash)
				
				chunkFile := fmt.Sprintf("%s.chunk_%d", filePath, chunk.StartByte)
				_ = os.WriteFile(chunkFile, []byte("done"), 0644)
				
				mu.Lock()
				bytesDone += chunk.EndByte - chunk.StartByte + 1
				job.BytesDownloaded = bytesDone
				s.Hub.Broadcast(ProgressEvent{
					JobID:         job.ID,
					TargetClients: job.TargetClients,
					Status:        "Downloading",
					BytesDone:     bytesDone,
					TotalSize:     job.TotalSize,
				})
				mu.Unlock()
			}
		}(c)
	}

	wg.Wait()

	if ctx.Err() != nil {
		log.Printf("[JOB %s] Cancelado/Pausado.", job.ID)
		return
	}

	if failedCount > 0 {
		log.Printf("[JOB %s] %d chunks falharam (primeiro erro: %v). Marcando como Error.", job.ID, failedCount, firstErr)
		_ = s.repo.UpdateJobStatus(job.ID, "Error")
		job.Status = "Error"
		s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Error", BytesDone: bytesDone, TotalSize: job.TotalSize})
		return
	}

	// --- Calcular SHA-256 do arquivo completo ---
	fileHash, err := computeFileSHA256(filePath)
	if err != nil {
		log.Printf("[JOB %s] Erro ao calcular SHA-256 do arquivo completo: %v", job.ID, err)
		_ = s.repo.UpdateJobStatus(job.ID, "Error")
		s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Error"})
		return
	}

	// Limpar marcadores de chunks
	cleanupChunkFiles(filePath)

	job.RootHash = fileHash
	_ = s.repo.UpdateRootHash(job.ID, fileHash)
	log.Printf("[JOB %s] Todos os chunks finalizados. SHA-256: %s. Marcando como Completed.", job.ID, fileHash)

	_ = s.repo.UpdateJobStatus(job.ID, "Completed")
	s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Completed", BytesDone: job.TotalSize, TotalSize: job.TotalSize})
}

func (s *DownloadService) PauseDownload(jobID string) error {
	s.activeMu.RLock()
	job, exists := s.activeJobs[jobID]
	s.activeMu.RUnlock()

	if !exists {
		return fmt.Errorf("job is not active")
	}

	if job.CancelFunc != nil {
		job.CancelFunc()
	}

	job.Status = "Paused"
	_ = s.repo.UpdateJobStatus(jobID, "Paused")
	s.Hub.Broadcast(ProgressEvent{JobID: jobID, TargetClients: job.TargetClients, Status: "Paused", TotalSize: job.TotalSize})

	return nil
}

func (s *DownloadService) ResumeDownload(jobID string) error {
	job, err := s.repo.FindJobByID(jobID)
	if err != nil {
		return err
	}

	if job.Status != "Paused" && job.Status != "Error" {
		return fmt.Errorf("job is not paused")
	}

	_ = s.repo.UpdateJobStatus(job.ID, "Queued")
	job.Status = "Queued"
	s.Hub.Broadcast(ProgressEvent{JobID: jobID, TargetClients: job.TargetClients, Status: "Queued", TotalSize: job.TotalSize})
	
	ctx, cancel := context.WithCancel(context.Background())
	job.CancelFunc = cancel

	go func() {
		s.activeDownloads <- struct{}{}
		defer func() { <-s.activeDownloads }()

		_ = s.repo.UpdateJobStatus(job.ID, "Downloading")
		job.Status = "Downloading"
		s.Hub.Broadcast(ProgressEvent{JobID: job.ID, TargetClients: job.TargetClients, Status: "Downloading", TotalSize: job.TotalSize})

		s.activeMu.Lock()
		s.activeJobs[job.ID] = job
		s.activeMu.Unlock()

		defer func() {
			s.activeMu.Lock()
			delete(s.activeJobs, job.ID)
			s.activeMu.Unlock()
		}()
		s.startChunks(ctx, job, true)
	}()

	return nil
}

// computeMerkleRoot constrói uma Merkle Tree dos hashes dos chunks
// e retorna o hash raiz como hex string.
// Se não houver chunks, retorna string vazia.
func computeMerkleRoot(chunks []models.ChunkTask) string {
	if len(chunks) == 0 {
		return ""
	}

	// Coleta hashes das folhas
	leaves := make([][]byte, len(chunks))
	for i, chunk := range chunks {
		decoded, err := hex.DecodeString(chunk.Hash)
		if err != nil || len(decoded) == 0 {
			// fallback: hash do ID do chunk
			h := sha256.Sum256([]byte(chunk.ID))
			decoded = h[:]
		}
		leaves[i] = decoded
	}

	// Constrói a árvore iterativamente
	layer := leaves
	for len(layer) > 1 {
		var next [][]byte
		for i := 0; i < len(layer); i += 2 {
			if i+1 < len(layer) {
				combined := append(layer[i], layer[i+1]...)
				h := sha256.Sum256(combined)
				next = append(next, h[:])
			} else {
				// número ímpar de nós: duplicar o último (padrão Bitcoin Merkle)
				combined := append(layer[i], layer[i]...)
				h := sha256.Sum256(combined)
				next = append(next, h[:])
			}
		}
		layer = next
	}

	return hex.EncodeToString(layer[0])
}
