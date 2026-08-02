package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"machdown-client/config"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type SyncJob struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	FileName  string `json:"file_name"`
	TotalSize int64  `json:"total_size"`
	Status    string `json:"status"` // Syncing, Completed, Error
	// RootHash é o hash SHA-256 do arquivo completo enviado pelo servidor.
	// Usado para validar a integridade do arquivo após o download (task pendente).
	RootHash string `json:"root_hash"`
}

type SyncDaemon struct {
	cfg        config.ClientConfig
	isRunning  bool
	stopChan   chan struct{}
	ActiveJobs []SyncJob
}

func NewSyncDaemon(cfg config.ClientConfig) *SyncDaemon {
	return &SyncDaemon{
		cfg:        cfg,
		stopChan:   make(chan struct{}),
		ActiveJobs: make([]SyncJob, 0),
	}
}

func (d *SyncDaemon) UpdateConfig(cfg config.ClientConfig) {
	d.cfg = cfg
}

func (d *SyncDaemon) Start() {
	if d.isRunning {
		return
	}
	d.isRunning = true
	go d.poll()
}

func (d *SyncDaemon) Stop() {
	if !d.isRunning {
		return
	}
	d.isRunning = false
	d.stopChan <- struct{}{}
}

func (d *SyncDaemon) GetJobs() []SyncJob {
	return d.ActiveJobs
}

func (d *SyncDaemon) poll() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if d.cfg.APIKey == "" || d.cfg.ServerURL == "" || d.cfg.ClientID == "" {
				continue
			}
			d.fetchCompletedJobs()
		case <-d.stopChan:
			log.Println("SyncDaemon: Parado")
			return
		}
	}
}

func (d *SyncDaemon) fetchCompletedJobs() {
	req, _ := http.NewRequest("GET", d.cfg.ServerURL+"/api/downloads/completed?client_id="+d.cfg.ClientID, nil)
	req.Header.Set("X-API-Key", d.cfg.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return
	}
	defer resp.Body.Close()

	var jobs []SyncJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return
	}

	d.ActiveJobs = jobs

	for _, job := range jobs {
		d.downloadFile(job)
	}
}

func (d *SyncDaemon) downloadFile(job SyncJob) {
	log.Printf("Iniciando sincronização do job %s (%s)", job.ID, job.FileName)

	// Determinar pasta base com base na URL
	basePath := d.cfg.DownloadPath
	if u, err := url.Parse(job.URL); err == nil && u.Hostname() != "" {
		host := u.Hostname()
		for domain, customPath := range d.cfg.SiteMappings {
			if host == domain || strings.HasSuffix(host, "."+domain) {
				basePath = customPath
				break
			}
		}
	}

	// Garantir que o diretório de download existe
	os.MkdirAll(basePath, 0755)

	// Inferir nome do arquivo
	finalName := job.FileName
	if finalName == "downloaded_file" {
		finalName = filepath.Base(job.URL)
	}

	destPath := filepath.Join(basePath, finalName)

	req, _ := http.NewRequest("GET", d.cfg.ServerURL+"/api/downloads/"+job.ID+"/file", nil)
	req.Header.Set("X-API-Key", d.cfg.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[JOB %s] Falha ao buscar arquivo do servidor: status=%v err=%v", job.ID, resp.StatusCode, err)
		return
	}
	defer resp.Body.Close()

	// Escrever em arquivo temporário primeiro para evitar arquivo corrompido em destino final
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[JOB %s] Falha ao criar arquivo temporário: %v", job.ID, err)
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
		return
	}
	out.Close()

	// Comparar hash recebido vs hash calculado
	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if job.RootHash != "" && job.RootHash != computedHash {
		os.Remove(tmpPath)
		log.Printf("[JOB %s] ⚠️  FALHA DE INTEGRIDADE! Hash esperado: %s | Hash calculado: %s", job.ID, job.RootHash, computedHash)
		return
	}
	if job.RootHash == "" {
		log.Printf("[JOB %s] ⚠️  Servidor não forneceu RootHash — pulando validação de integridade.", job.ID)
	}

	// Mover do temporário para o destino final apenas se o hash for válido
	if err := atomicMove(tmpPath, destPath); err != nil {
		log.Printf("[JOB %s] Falha ao mover arquivo para destino final: %v", job.ID, err)
		return
	}

	log.Printf("[JOB %s] ✅ Arquivo salvo em %s (hash: %s)", job.ID, destPath, computedHash)

	// Marcar como sincronizado no servidor para não re-baixar
	markReq, _ := http.NewRequest("POST", d.cfg.ServerURL+"/api/downloads/"+job.ID+"/synced", nil)
	markReq.Header.Set("X-API-Key", d.cfg.APIKey)
	client.Do(markReq)
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
