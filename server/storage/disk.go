package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type DiskManager struct {
	basePath string
}

func NewDiskManager(basePath string) *DiskManager {
	return &DiskManager{basePath: basePath}
}

// AllocateFile creates a sparse file of the given total size
func (d *DiskManager) AllocateFile(jobID string, category string, totalSize int64) (string, error) {
	categoryFolder := category
	if categoryFolder == "" {
		categoryFolder = "Others"
	}
	dirPath := filepath.Join(d.basePath, categoryFolder)
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		return "", err
	}

	safeID := filepath.Base(jobID)
	absBase, err := filepath.Abs(d.basePath)
	if err != nil {
		return "", err
	}
	path := filepath.Join(absBase, categoryFolder, safeID)
	rel, err := filepath.Rel(absBase, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("security error: path traversal detected")
	}
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

// GetFilePath returns the absolute path for a given jobID and category
func (d *DiskManager) GetFilePath(jobID string, category string) string {
	categoryFolder := category
	if categoryFolder == "" {
		categoryFolder = "Others"
	}
	safeID := filepath.Base(jobID)
	absBase, err := filepath.Abs(d.basePath)
	if err != nil {
		return ""
	}
	path := filepath.Join(absBase, categoryFolder, safeID)
	rel, err := filepath.Rel(absBase, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return path
}

// FindFilePath locates the file for jobID by checking root and category subdirectories.
func (d *DiskManager) FindFilePath(jobID string) string {
	safeID := filepath.Base(jobID)
	absBase, err := filepath.Abs(d.basePath)
	if err != nil {
		return ""
	}

	// 1. Direct path in root
	direct := filepath.Join(absBase, safeID)
	if _, err := os.Stat(direct); err == nil {
		return direct
	}

	// 2. Look in subdirectories (category folders)
	entries, err := os.ReadDir(absBase)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				candidate := filepath.Join(absBase, entry.Name(), safeID)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}

	// Fallback to Others
	return filepath.Join(absBase, "Others", safeID)
}

// DeleteJobFiles removes the main file and any .chunk_* marker files for a given jobID.
func (d *DiskManager) DeleteJobFiles(jobID string) error {
	filePath := d.FindFilePath(jobID)
	if filePath != "" {
		_ = os.Remove(filePath)
		
		// Remove marker files
		dir := filepath.Dir(filePath)
		safeID := filepath.Base(jobID)
		matches, err := filepath.Glob(filepath.Join(dir, safeID+".chunk_*"))
		if err == nil {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}
	return nil
}

type StoredFile struct {
	ID       string
	Category string
	Path     string
	Size     int64
}

// ListAllStoredFiles lists all actual downloaded files across all category directories.
func (d *DiskManager) ListAllStoredFiles() ([]StoredFile, error) {
	absBase, err := filepath.Abs(d.basePath)
	if err != nil {
		return nil, err
	}

	var results []StoredFile
	err = filepath.WalkDir(absBase, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}

		name := d.Name()
		// Ignore chunk marker files or hidden files
		if strings.Contains(name, ".chunk_") || strings.HasPrefix(name, ".") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(absBase, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		category := ""
		id := name
		if len(parts) > 1 {
			category = parts[0]
			id = parts[len(parts)-1]
		}

		results = append(results, StoredFile{
			ID:       id,
			Category: category,
			Path:     path,
			Size:     info.Size(),
		})
		return nil
	})

	return results, err
}

