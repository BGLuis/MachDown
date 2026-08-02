package storage

import (
	"os"
	"path/filepath"
)

type DiskManager struct {
	basePath string
}

func NewDiskManager(basePath string) *DiskManager {
	return &DiskManager{basePath: basePath}
}

// AllocateFile creates a sparse file of the given total size
func (d *DiskManager) AllocateFile(jobID string, totalSize int64) (string, error) {
	err := os.MkdirAll(d.basePath, 0755)
	if err != nil {
		return "", err
	}

	path := filepath.Join(d.basePath, jobID)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	
	// Create sparse file (allocates logical size without using all physical disk space yet)
	if err := f.Truncate(totalSize); err != nil {
		return "", err
	}
	
	return path, nil
}

// GetFilePath returns the absolute path for a given jobID
func (d *DiskManager) GetFilePath(jobID string) string {
	return filepath.Join(d.basePath, jobID)
}
