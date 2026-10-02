package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ServerFileInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Status string `json:"status"`
}

type ServerConfigDTO struct {
	APIKey                 string `json:"api_key"`
	StoragePath            string `json:"storage_path"`
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
}

// doAdminRequest is a helper to get base url and api key and make requests
func (a *App) doAdminRequest(method, endpoint string, body io.Reader) (*http.Response, error) {
	cfg := a.GetConfig()
	if cfg.ServerURL == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("Client not configured")
	}

	req, err := http.NewRequest(method, cfg.ServerURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", cfg.APIKey)
	req.Header.Set("X-Client-ID", cfg.ClientID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	return client.Do(req)
}

// ListServerFiles fetches the list of files in the server's storage
func (a *App) ListServerFiles() ([]ServerFileInfo, error) {
	resp, err := a.doAdminRequest("GET", "/api/admin/files", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status %d", resp.StatusCode)
	}

	var files []ServerFileInfo
	err = json.NewDecoder(resp.Body).Decode(&files)
	return files, err
}

// DeleteServerFile deletes a file from the server
func (a *App) DeleteServerFile(filename string) error {
	resp, err := a.doAdminRequest("DELETE", "/api/admin/files/"+filename, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %d", resp.StatusCode)
	}
	return nil
}

// GetServerConfig fetches the server configuration
func (a *App) GetServerConfig() (*ServerConfigDTO, error) {
	resp, err := a.doAdminRequest("GET", "/api/admin/config", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status %d", resp.StatusCode)
	}

	var config ServerConfigDTO
	err = json.NewDecoder(resp.Body).Decode(&config)
	return &config, err
}

// UpdateServerConfig updates the server configuration
func (a *App) UpdateServerConfig(config ServerConfigDTO) (*ServerConfigDTO, error) {
	bodyData, _ := json.Marshal(config)
	resp, err := a.doAdminRequest("PUT", "/api/admin/config", bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status %d", resp.StatusCode)
	}

	var updatedConfig ServerConfigDTO
	err = json.NewDecoder(resp.Body).Decode(&updatedConfig)
	return &updatedConfig, err
}

// CleanServer requests the server to clean up error jobs and orphan files
func (a *App) CleanServer() (string, error) {
	resp, err := a.doAdminRequest("POST", "/api/admin/clean", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Status %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["message"], nil
}

// DeleteBatchServerFiles deletes multiple files from the server
func (a *App) DeleteBatchServerFiles(ids []string) error {
	bodyData, _ := json.Marshal(map[string][]string{"ids": ids})
	resp, err := a.doAdminRequest("POST", "/api/admin/files/delete-batch", bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %d", resp.StatusCode)
	}
	return nil
}

// ForceSyncBatchServerFiles forces the server to assign these jobs to this client so they are downloaded
func (a *App) ForceSyncBatchServerFiles(ids []string) error {
	cfg := a.GetConfig()
	bodyData, _ := json.Marshal(map[string]interface{}{
		"ids":       ids,
		"client_id": cfg.ClientID,
	})
	resp, err := a.doAdminRequest("POST", "/api/admin/files/sync-batch", bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %d", resp.StatusCode)
	}
	return nil
}

type APIKeyDTO struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ListAPIKeys fetches the list of API Keys from the server
func (a *App) ListAPIKeys() ([]APIKeyDTO, error) {
	resp, err := a.doAdminRequest("GET", "/api/admin/apikeys", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status %d", resp.StatusCode)
	}

	var keys []APIKeyDTO
	err = json.NewDecoder(resp.Body).Decode(&keys)
	return keys, err
}

// CreateAPIKey creates a new API Key on the server
func (a *App) CreateAPIKey(name, key string) error {
	bodyData, _ := json.Marshal(map[string]string{"name": name, "key": key})
	resp, err := a.doAdminRequest("POST", "/api/admin/apikeys", bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %d", resp.StatusCode)
	}
	return nil
}

// DeleteAPIKey deletes an API Key from the server
func (a *App) DeleteAPIKey(id string) error {
	resp, err := a.doAdminRequest("DELETE", "/api/admin/apikeys/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %d", resp.StatusCode)
	}
	return nil
}

type ClientDTO struct {
	ID       string `json:"id"`
	LastSeen string `json:"last_seen"`
	IP       string `json:"ip"`
}

// ListClients fetches the list of connected clients from the server
func (a *App) ListClients() ([]ClientDTO, error) {
	resp, err := a.doAdminRequest("GET", "/api/admin/clients", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status %d", resp.StatusCode)
	}

	var clients []ClientDTO
	err = json.NewDecoder(resp.Body).Decode(&clients)
	return clients, err
}
