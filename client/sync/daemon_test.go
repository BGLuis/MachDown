package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"machdown-client/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicMoveAndHashValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "machdown-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("Hello MachDown client integrity validation test content!")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	src := filepath.Join(tempDir, "test.tmp")
	dst := filepath.Join(tempDir, "test.final")

	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := atomicMove(src, dst); err != nil {
		t.Fatalf("atomicMove failed: %v", err)
	}

	readBack, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("Failed to read back destination file: %v", err)
	}

	h := sha256.Sum256(readBack)
	readHash := hex.EncodeToString(h[:])
	if readHash != expectedHash {
		t.Errorf("Expected hash %s, got %s", expectedHash, readHash)
	}
}

func TestPathTraversalSanitization(t *testing.T) {
	basePath := "/home/user/Downloads/MachDown"

	maliciousNames := []string{
		"../../etc/passwd",
		"../../../root/.bashrc",
		"subdir/../../cron.d/evil",
		"normal_file.zip",
	}

	for _, name := range maliciousNames {
		safeName := filepath.Base(name)
		destPath := filepath.Join(basePath, safeName)
		rel, err := filepath.Rel(basePath, destPath)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || rel[:2] == ".." {
			t.Errorf("Path traversal escaped for %s -> %s", name, destPath)
		}
	}
}

func newDaemonFor(t *testing.T, srv *httptest.Server, fingerprint string) *SyncDaemon {
	t.Helper()
	return NewSyncDaemon(config.ClientConfig{
		ServerURL:             srv.URL,
		APIKey:                "key",
		ClientID:              "pc",
		DownloadPath:          t.TempDir(),
		ServerCertFingerprint: fingerprint,
	})
}

func TestDownloadFileWithUnreachableServerMarksErrorWithoutPanic(t *testing.T) {
	d := NewSyncDaemon(config.ClientConfig{
		ServerURL:    "https://127.0.0.1:1",
		APIKey:       "key",
		ClientID:     "pc",
		DownloadPath: t.TempDir(),
	})
	job := &SyncJob{ID: "j1", FileName: "a.bin"}

	d.downloadFile(context.Background(), d.config(), job)

	if job.Status != "Error" {
		t.Fatalf("expected Error status, got %q", job.Status)
	}
}

func TestSyncOverPinnedTLSDownloadsAndValidatesHash(t *testing.T) {
	content := []byte("payload from a self-signed server")
	sum := sha256.Sum256(content)
	rootHash := hex.EncodeToString(sum[:])

	synced := make(chan struct{}, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/downloads/completed":
			json.NewEncoder(w).Encode([]SyncJob{{ID: "j1", FileName: "a.bin", TotalSize: int64(len(content)), RootHash: rootHash}})
		case r.URL.Path == "/api/downloads/j1/file":
			w.Write(content)
		case r.URL.Path == "/api/downloads/j1/synced":
			synced <- struct{}{}
		}
	}))
	defer srv.Close()

	certSum := sha256.Sum256(srv.Certificate().Raw)
	d := newDaemonFor(t, srv, hex.EncodeToString(certSum[:]))

	d.fetchCompletedJobs(context.Background(), d.config())

	got, err := os.ReadFile(filepath.Join(d.config().DownloadPath, "a.bin"))
	if err != nil || string(got) != string(content) {
		t.Fatalf("file not synced correctly: %v %q", err, got)
	}
	if jobs := d.GetJobs(); len(jobs) != 1 || jobs[0].Status != "Completed" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
	select {
	case <-synced:
	default:
		t.Fatal("server was not told the job was synced")
	}
}

func TestSyncRejectsCorruptedFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/downloads/completed":
			json.NewEncoder(w).Encode([]SyncJob{{ID: "j1", FileName: "a.bin", RootHash: "deadbeef"}})
		case "/api/downloads/j1/file":
			w.Write([]byte("tampered"))
		}
	}))
	defer srv.Close()

	certSum := sha256.Sum256(srv.Certificate().Raw)
	d := newDaemonFor(t, srv, hex.EncodeToString(certSum[:]))

	d.fetchCompletedJobs(context.Background(), d.config())

	if _, err := os.Stat(filepath.Join(d.config().DownloadPath, "a.bin")); err == nil {
		t.Fatal("a file failing the hash check must not reach its final path")
	}
	if jobs := d.GetJobs(); len(jobs) != 1 || jobs[0].Status != "Error" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}

func TestGetJobsIsSafeDuringSync(t *testing.T) {
	d := NewSyncDaemon(config.ClientConfig{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			d.setJobs([]SyncJob{{ID: "j", Status: "Syncing"}})
			d.UpdateConfig(config.ClientConfig{ClientID: "pc"})
		}
	}()
	for i := 0; i < 500; i++ {
		_ = d.GetJobs()
		_ = d.config()
	}
	<-done
}

func TestStopAbortsOpenSSEStream(t *testing.T) {
	connected := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/events" {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(connected)
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	certSum := sha256.Sum256(srv.Certificate().Raw)
	d := newDaemonFor(t, srv, hex.EncodeToString(certSum[:]))
	d.Start()
	<-connected

	stopped := make(chan struct{})
	go func() { d.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked while the SSE stream was idle")
	}
}
