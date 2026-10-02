package storage

import (
	"os"
	"testing"
)

func TestDiskManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "machdown-disk-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dm := NewDiskManager(tempDir)

	// 1. AllocateFile
	jobID := "test-job-uuid-1234"
	category := "Videos"
	size := int64(1024)

	allocatedPath, err := dm.AllocateFile(jobID, category, size)
	if err != nil {
		t.Fatalf("AllocateFile failed: %v", err)
	}

	info, err := os.Stat(allocatedPath)
	if err != nil {
		t.Fatalf("Stat allocated file failed: %v", err)
	}
	if info.Size() != size {
		t.Errorf("Expected size %d, got %d", size, info.Size())
	}

	// 2. FindFilePath
	foundPath := dm.FindFilePath(jobID)
	if foundPath != allocatedPath {
		t.Errorf("FindFilePath expected %s, got %s", allocatedPath, foundPath)
	}

	// 3. Create a fake chunk file
	chunkFile := allocatedPath + ".chunk_0"
	if err := os.WriteFile(chunkFile, []byte("done"), 0644); err != nil {
		t.Fatalf("Failed to write chunk file: %v", err)
	}

	// 4. ListAllStoredFiles (should NOT list .chunk_ files)
	stored, err := dm.ListAllStoredFiles()
	if err != nil {
		t.Fatalf("ListAllStoredFiles failed: %v", err)
	}
	if len(stored) != 1 {
		t.Errorf("Expected 1 stored file, found %d", len(stored))
	}
	if len(stored) > 0 && stored[0].ID != jobID {
		t.Errorf("Expected ID %s, got %s", jobID, stored[0].ID)
	}

	// 5. DeleteJobFiles
	if err := dm.DeleteJobFiles(jobID); err != nil {
		t.Fatalf("DeleteJobFiles failed: %v", err)
	}
	if _, err := os.Stat(allocatedPath); !os.IsNotExist(err) {
		t.Errorf("Allocated file was not deleted")
	}
	if _, err := os.Stat(chunkFile); !os.IsNotExist(err) {
		t.Errorf("Chunk marker file was not deleted")
	}

	// 6. Path traversal rejection
	_, err = dm.AllocateFile("../../evil", "Videos", 100)
	// safeID is filepath.Base("../../evil") = "evil", which stays inside tempDir/Videos/evil
	// But if someone tries relative traversal with category:
	_, err = dm.AllocateFile("safe-id", "../../etc", 100)
	if err == nil {
		t.Errorf("Expected error for path traversal category, got nil")
	}
}
