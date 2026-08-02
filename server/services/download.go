package services

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"machdown/server/models"
	"machdown/server/repositories"
	"machdown/server/storage"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
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
}

func NewDownloadService(repo *repositories.JobRepository, cd *ChunkDownloader, disk *storage.DiskManager, hub *ProgressHub) *DownloadService {
	return &DownloadService{
		repo:       repo,
		downloader: cd,
		disk:       disk,
		Hub:        hub,
		resolver:   NewLinkResolver(),
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
	return s.disk.GetFilePath(jobID)
}


func (s *DownloadService) Enqueue(rawURL string, targetClients string, cookies string, userAgent string) (*models.DownloadJob, error) {
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

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			if len(via) > 0 {
				if cookie := via[0].Header.Get("Cookie"); cookie != "" {
					req.Header.Set("Cookie", cookie)
				}
				if ua := via[0].Header.Get("User-Agent"); ua != "" {
					req.Header.Set("User-Agent", ua)
				}
			}
			return nil
		},
	}
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
		TotalSize:     totalSize,
		ETag:          etag,
		Status:        "Pending",
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
	log.Printf("[JOB %s] Iniciando processamento.", job.ID)
	_ = s.repo.UpdateJobStatus(job.ID, "Downloading")
	s.Hub.Broadcast(ProgressEvent{JobID: job.ID, Status: "Downloading", TotalSize: job.TotalSize})

	if job.TotalSize == 0 {
		log.Printf("[JOB %s] Tamanho desconhecido (sem Content-Length). Usando download sequencial.", job.ID)
		err := s.downloader.DownloadSequential(job)
		if err != nil {
			log.Printf("[JOB %s] Download sequencial falhou: %v", job.ID, err)
			_ = s.repo.UpdateJobStatus(job.ID, "Error")
			s.Hub.Broadcast(ProgressEvent{JobID: job.ID, Status: "Error"})
		} else {
			log.Printf("[JOB %s] Download sequencial concluído.", job.ID)
			
			// O nome do arquivo pode ter sido descoberto via Content-Disposition durante o download
			_ = s.repo.UpdateJobFileName(job.ID, job.FileName)
			
			_ = s.repo.UpdateJobStatus(job.ID, "Completed")
			s.Hub.Broadcast(ProgressEvent{JobID: job.ID, Status: "Completed", BytesDone: job.TotalSize, TotalSize: job.TotalSize})
		}
		return
	}

	// Alocar arquivo esparso
	_, err := s.disk.AllocateFile(job.ID, job.TotalSize)
	if err != nil {
		log.Printf("[JOB %s] Erro ao alocar arquivo esparso: %v", job.ID, err)
		_ = s.repo.UpdateJobStatus(job.ID, "Error")
		s.Hub.Broadcast(ProgressEvent{JobID: job.ID, Status: "Error"})
		return
	}

	// Dividir em chunks
	var chunks []models.ChunkTask
	for i := int64(0); i < job.TotalSize; i += chunkSize {
		end := i + chunkSize - 1
		if end >= job.TotalSize {
			end = job.TotalSize - 1
		}
		chunks = append(chunks, models.ChunkTask{
			ID:        uuid.New().String(),
			JobID:     job.ID,
			StartByte: i,
			EndByte:   end,
			Status:    "Pending",
		})
	}

	log.Printf("[JOB %s] Arquivo alocado. Dividindo em %d chunks. Iniciando workers...", job.ID, len(chunks))

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		bytesDone int64
	)
	sem := make(chan struct{}, 8)

	for _, c := range chunks {
		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(chunk models.ChunkTask) {
			defer wg.Done()
			defer func() { <-sem }() // release

			err := s.downloader.DownloadChunk(job, &chunk)
			if err != nil {
				log.Printf("[JOB %s] Chunk %d-%d falhou: %v", job.ID, chunk.StartByte, chunk.EndByte, err)
			} else {
				log.Printf("[JOB %s] Chunk %d-%d concluído com hash: %s", job.ID, chunk.StartByte, chunk.EndByte, chunk.Hash)
				mu.Lock()
				bytesDone += chunk.EndByte - chunk.StartByte + 1
				s.Hub.Broadcast(ProgressEvent{
					JobID:     job.ID,
					Status:    "Downloading",
					BytesDone: bytesDone,
					TotalSize: job.TotalSize,
				})
				mu.Unlock()
			}
		}(c)
	}

	wg.Wait()

	// --- Merkle Tree: calcular RootHash a partir dos hashes dos chunks ---
	rootHash := computeMerkleRoot(chunks)
	_ = s.repo.UpdateRootHash(job.ID, rootHash)
	log.Printf("[JOB %s] Todos os chunks finalizados. RootHash Merkle: %s. Marcando como Completed.", job.ID, rootHash)

	_ = s.repo.UpdateJobStatus(job.ID, "Completed")
	s.Hub.Broadcast(ProgressEvent{JobID: job.ID, Status: "Completed", BytesDone: job.TotalSize, TotalSize: job.TotalSize})
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
