package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
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
