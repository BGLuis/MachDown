package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

type DownloadJob struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	Priority      int       `json:"priority"`
	URL           string    `json:"url"`
	FileName      string    `json:"file_name"`
	Category      string    `json:"category"`
	TotalSize       int64              `json:"total_size"`
	Status          string             `json:"status"` // Pending, Downloading, Paused, Completed, Error
	State           string             `json:"state"`
	BytesDownloaded int64              `json:"bytes_downloaded"`
	CancelFunc      context.CancelFunc `gorm:"-" json:"-"`
	ETag            string             `json:"etag"` // HTTP ETag para deduplicação
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
	APIKey                 string `json:"-"` // legacy plaintext column; migrated to the hashed APIKey table and cleared at startup
	StoragePath            string `json:"storage_path"`
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
}

type APIKey struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	KeyHash   string    `json:"-"`
	KeyPrefix string    `json:"key_prefix"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func GenerateAPIKeyData(rawKey string) (id string, keyHash string, prefix string) {
	hash256 := sha256.Sum256([]byte(rawKey))
	id = hex.EncodeToString(hash256[:])

	if len(rawKey) >= 8 {
		prefix = rawKey[:8]
	} else {
		prefix = rawKey
	}

	salt := make([]byte, 16)
	rand.Read(salt)
	hashArgon := argon2.IDKey([]byte(rawKey), salt, 1, 64*1024, 4, 32)
	
	keyHash = hex.EncodeToString(salt) + "." + hex.EncodeToString(hashArgon)
	return id, keyHash, prefix
}

func VerifyAPIKey(rawKey, keyHash string) bool {
	parts := strings.Split(keyHash, ".")
	if len(parts) != 2 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[0])
	expectedHash, err2 := hex.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	hashArgon := argon2.IDKey([]byte(rawKey), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(hashArgon, expectedHash) == 1
}

type Client struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	LastSeen  time.Time `json:"last_seen"`
	IP        string    `json:"ip"`
}
