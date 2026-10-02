package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"machdown/server/models"
	"machdown/server/storage"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ChunkDownloader struct {
	diskManager *storage.DiskManager
}

func NewChunkDownloader(dm *storage.DiskManager) *ChunkDownloader {
	return &ChunkDownloader{diskManager: dm}
}

func (c *ChunkDownloader) DownloadChunk(ctx context.Context, job *models.DownloadJob, chunk *models.ChunkTask) error {
	log.Printf("[JOB %s] Chunk %d-%d: Requesting GET %s", job.ID, chunk.StartByte, chunk.EndByte, job.URL)
	req, err := http.NewRequestWithContext(ctx, "GET", job.URL, nil)
	if err != nil {
		return err
	}

	if job.Cookies != "" {
		req.Header.Set("Cookie", job.Cookies)
	}
	if job.UserAgent != "" {
		req.Header.Set("User-Agent", job.UserAgent)
	}

	// Request specific byte range
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.StartByte, chunk.EndByte))

	client := GetSharedHTTPClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
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
	}
	resp, err := RetryDownload(func() (*http.Response, error) {
		return client.Do(req)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Rejeitar respostas HTML: indica página de verificação (Cloudflare, anti-bot, etc.)
	// em vez do arquivo real.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		return fmt.Errorf("chunk download recebeu HTML em vez de arquivo (content-type: %s). URL pode precisar de resolução de link direto", contentType)
	}

	log.Printf("[JOB %s] Chunk %d-%d: Got response, saving to disk...", job.ID, chunk.StartByte, chunk.EndByte)

	// Open the sparse file to write this chunk
	filePath := c.diskManager.GetFilePath(job.ID, "")
	f, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Seek to the start byte
	if _, err := f.Seek(chunk.StartByte, io.SeekStart); err != nil {
		return err
	}

	// Setup hashing for Merkle Tree leaves
	hasher := sha256.New()

	// Create a TeeReader that writes to the hasher while reading from resp.Body
	tee := io.TeeReader(resp.Body, hasher)

	// Copy from tee to file
	// Limit the read to exactly what we requested
	limitReader := io.LimitReader(tee, chunk.EndByte-chunk.StartByte+1)

	if _, err := io.Copy(f, limitReader); err != nil {
		return err
	}

	chunk.Hash = hex.EncodeToString(hasher.Sum(nil))
	chunk.Status = "Completed"
	return nil
}

func (c *ChunkDownloader) DownloadSequential(ctx context.Context, job *models.DownloadJob) error {
	req, err := http.NewRequestWithContext(ctx, "GET", job.URL, nil)
	if err != nil {
		return err
	}

	if job.Cookies != "" {
		req.Header.Set("Cookie", job.Cookies)
	}
	if job.UserAgent != "" {
		req.Header.Set("User-Agent", job.UserAgent)
	}

	client := GetSharedHTTPClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
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
	}
	resp, err := RetryDownload(func() (*http.Response, error) {
		return client.Do(req)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Rejeitar respostas HTML: indica página de verificação (Cloudflare, anti-bot, etc.)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		return fmt.Errorf("download sequencial recebeu HTML em vez de arquivo (content-type: %s). URL pode precisar de resolução de link direto", contentType)
	}

	filePath := c.diskManager.GetFilePath(job.ID, "")
	os.MkdirAll(filepath.Dir(filePath), 0755)

	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		_, params, parseErr := mime.ParseMediaType(cd)
		if parseErr == nil && params["filename"] != "" {
			job.FileName = params["filename"]
			log.Printf("[JOB %s] Nome de arquivo real descoberto: %s", job.ID, job.FileName)
		}
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	log.Printf("[JOB %s] Sequential download wrote bytes, error: %v", job.ID, err)
	return err
}

// RetryDownload retries the given operation with exponential backoff and jitter.
func RetryDownload(operation func() (*http.Response, error)) (*http.Response, error) {
	var resp *http.Response
	var err error
	maxRetries := 3
	baseDelay := 1 * time.Second

	for i := 0; i <= maxRetries; i++ {
		resp, err = operation()
		if err == nil {
			return resp, nil
		}

		if i == maxRetries {
			break
		}

		delay := baseDelay * (1 << i)
		jitter := time.Duration(rand.Int63n(int64(delay)/5 + 1))
		time.Sleep(delay + jitter)
		log.Printf("Retry %d/%d after %v due to error: %v", i+1, maxRetries, delay+jitter, err)
	}

	return resp, err
}
