package models

import (
	"time"
)

type DownloadJob struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	URL           string    `json:"url"`
	FileName      string    `json:"file_name"`
	TotalSize     int64     `json:"total_size"`
	Status        string    `json:"status"` // Pending, Downloading, Paused, Completed, Error
	ETag          string    `json:"etag"`   // HTTP ETag para deduplicação
	RootHash      string    `json:"root_hash"`
	TargetClients string    `json:"target_clients"` // JSON array string de client IDs
	Cookies       string    `json:"cookies"`
	UserAgent     string    `json:"user_agent"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ChunkTask struct {
	ID        string `gorm:"primaryKey" json:"id"`
	JobID     string `gorm:"index" json:"job_id"`
	StartByte int64  `json:"start_byte"`
	EndByte   int64  `json:"end_byte"`
	Status    string `json:"status"` // Pending, Downloading, Completed, Error
	Hash      string `json:"hash"`
}

type ServerConfig struct {
	ID                     uint   `gorm:"primaryKey"`
	APIKey                 string `json:"api_key"` // DEPRECATED: use APIKey table
	StoragePath            string `json:"storage_path"`
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
}

type APIKey struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Client struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	LastSeen  time.Time `json:"last_seen"`
	IP        string    `json:"ip"`
}
