package sync

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"machdown-client/config"
	"machdown-client/netutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncJob struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	FileName        string `json:"file_name"`
	TotalSize       int64  `json:"total_size"`
	BytesDownloaded int64  `json:"downloaded"`
	Status          string `json:"status"` // Syncing, Completed, Error
	RootHash        string `json:"root_hash"`
}

type SyncDaemon struct {
	mu         sync.RWMutex
	cfg        config.ClientConfig
	cancel     context.CancelFunc
	activeJobs []SyncJob
}

func NewSyncDaemon(cfg config.ClientConfig) *SyncDaemon {
	return &SyncDaemon{
		cfg:        cfg,
		activeJobs: make([]SyncJob, 0),
	}
}

func (d *SyncDaemon) UpdateConfig(cfg config.ClientConfig) {
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
}

func (d *SyncDaemon) config() config.ClientConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

func (d *SyncDaemon) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go d.poll(ctx)
}

// Stop cancels the daemon; in-flight requests (including the SSE stream) are aborted.
func (d *SyncDaemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel == nil {
		return
	}
	d.cancel()
	d.cancel = nil
}

// GetJobs returns a snapshot of the current sync jobs.
func (d *SyncDaemon) GetJobs() []SyncJob {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]SyncJob(nil), d.activeJobs...)
}

func (d *SyncDaemon) setJobs(jobs []SyncJob) {
	snapshot := append([]SyncJob(nil), jobs...)
	d.mu.Lock()
	d.activeJobs = snapshot
	d.mu.Unlock()
}

func sleepCtx(ctx context.Context, wait time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

func (d *SyncDaemon) poll(ctx context.Context) {
	defer log.Println("SyncDaemon: Parado")
	for ctx.Err() == nil {
		cfg := d.config()
		if cfg.APIKey == "" || cfg.ServerURL == "" || cfg.ClientID == "" {
			sleepCtx(ctx, 5*time.Second)
			continue
		}
		d.listenSSE(ctx, cfg)
	}
}

func (d *SyncDaemon) listenSSE(ctx context.Context, cfg config.ClientConfig) {
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.ServerURL+"/api/events", nil)
	if err != nil {
		sleepCtx(ctx, 5*time.Second)
		return
	}
	req.Header.Set("X-API-Key", cfg.APIKey)
	req.Header.Set("X-Client-ID", cfg.ClientID)

	resp, err := netutil.Client(cfg.ServerCertFingerprint).Do(req)
	if err != nil {
		log.Printf("SSE request failed. Error: %v", err)
		sleepCtx(ctx, 5*time.Second)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("SSE request failed. Status: %v", resp.StatusCode)
		sleepCtx(ctx, 5*time.Second)
		return
	}

	// Jobs completed while the stream was down produce no event, so catch up on connect.
	d.fetchCompletedJobs(ctx, cfg)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var evt map[string]interface{}
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				if status, ok := evt["status"].(string); ok && status == "Completed" {
					d.fetchCompletedJobs(ctx, cfg)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		log.Printf("SSE scanner error: %v", err)
	}
	sleepCtx(ctx, 2*time.Second)
}

func (d *SyncDaemon) fetchCompletedJobs(ctx context.Context, cfg config.ClientConfig) {
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.ServerURL+"/api/downloads/completed?client_id="+url.QueryEscape(cfg.ClientID), nil)
	if err != nil {
		return
	}
	req.Header.Set("X-API-Key", cfg.APIKey)

	resp, err := netutil.Client(cfg.ServerCertFingerprint).Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var jobs []SyncJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return
	}

	d.setJobs(jobs)

	for i := range jobs {
		jobs[i].Status = "Syncing"
		d.setJobs(jobs)
		d.downloadFile(ctx, cfg, &jobs[i])
		d.setJobs(jobs)
	}
}

func (d *SyncDaemon) downloadFile(ctx context.Context, cfg config.ClientConfig, job *SyncJob) {
	log.Printf("Iniciando sincronização do job %s (%s)", job.ID, job.FileName)

	// Determinar pasta base com base na URL
	basePath := cfg.DownloadPath
	if u, err := url.Parse(job.URL); err == nil && u.Hostname() != "" {
		host := u.Hostname()
		for domain, customPath := range cfg.SiteMappings {
			if host == domain || strings.HasSuffix(host, "."+domain) {
				basePath = customPath
				break
			}
		}
	}

	// Garantir que o diretório de download existe
	os.MkdirAll(basePath, 0755)

	// Inferir nome do arquivo e sanitizar contra Path Traversal
	finalName := filepath.Base(job.FileName)
	if finalName == "." || finalName == "/" || finalName == "" || finalName == "downloaded_file" {
		finalName = filepath.Base(job.URL)
	}
	if finalName == "." || finalName == "/" || finalName == "" {
		finalName = "downloaded_file_" + job.ID
	}

	destPath := filepath.Join(basePath, finalName)
	rel, relErr := filepath.Rel(basePath, destPath)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		log.Printf("[JOB %s] Security error: path traversal detectado para %s", job.ID, finalName)
		job.Status = "Error"
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", cfg.ServerURL+"/api/downloads/"+job.ID+"/file", nil)
	if err != nil {
		log.Printf("[JOB %s] Falha ao montar requisição: %v", job.ID, err)
		job.Status = "Error"
		return
	}
	req.Header.Set("X-API-Key", cfg.APIKey)

	client := netutil.Client(cfg.ServerCertFingerprint)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[JOB %s] Falha ao buscar arquivo do servidor: %v", job.ID, err)
		job.Status = "Error"
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[JOB %s] Falha ao buscar arquivo do servidor: status=%d", job.ID, resp.StatusCode)
		job.Status = "Error"
		return
	}

	// Escrever em arquivo temporário primeiro para evitar arquivo corrompido em destino final
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[JOB %s] Falha ao criar arquivo temporário: %v", job.ID, err)
		job.Status = "Error"
		return
	}

	// --- Validação de Integridade SHA-256 ---
	// Computamos o hash ao mesmo tempo que escrevemos, sem segunda leitura do arquivo.
	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)

	if _, err = io.Copy(out, tee); err != nil {
		out.Close()
		os.Remove(tmpPath)
		log.Printf("[JOB %s] Falha ao salvar arquivo local: %v", job.ID, err)
		job.Status = "Error"
		return
	}
	out.Close()

	// Comparar hash recebido vs hash calculado
	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if job.RootHash != "" && job.RootHash != computedHash {
		os.Remove(tmpPath)
		log.Printf("[JOB %s] ⚠️  FALHA DE INTEGRIDADE! Hash esperado: %s | Hash calculado: %s", job.ID, job.RootHash, computedHash)
		job.Status = "Error"
		return
	}
	if job.RootHash == "" {
		log.Printf("[JOB %s] ⚠️  Servidor não forneceu RootHash — pulando validação de integridade.", job.ID)
	}

	// Mover do temporário para o destino final apenas se o hash for válido
	if err := atomicMove(tmpPath, destPath); err != nil {
		log.Printf("[JOB %s] Falha ao mover arquivo para destino final: %v", job.ID, err)
		job.Status = "Error"
		return
	}

	job.Status = "Completed"
	job.BytesDownloaded = job.TotalSize
	log.Printf("[JOB %s] ✅ Arquivo salvo em %s (hash: %s)", job.ID, destPath, computedHash)

	// Marcar como sincronizado no servidor para não re-baixar
	markReq, err := http.NewRequestWithContext(ctx, "POST", cfg.ServerURL+"/api/downloads/"+job.ID+"/synced", nil)
	if err != nil {
		return
	}
	markReq.Header.Set("X-API-Key", cfg.APIKey)
	markResp, err := client.Do(markReq)
	if err != nil {
		log.Printf("[JOB %s] Falha ao marcar como sincronizado: %v", job.ID, err)
		return
	}
	markResp.Body.Close()
}

// atomicMove move um arquivo de src para dst.
// Tenta rename (atômico no mesmo filesystem), com fallback para copy+delete.
func atomicMove(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fallback: copy + delete (cross-device ou permissões)
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("atomicMove open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("atomicMove create dst: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		os.Remove(dst)
		return fmt.Errorf("atomicMove copy: %w", err)
	}
	os.Remove(src)
	return nil
}
