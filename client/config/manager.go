package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ClientConfig struct {
	ServerURL    string `json:"server_url"`
	APIKey       string `json:"api_key"`
	ClientID     string            `json:"client_id"`
	DownloadPath string            `json:"download_path"`
	SiteMappings map[string]string `json:"site_mappings,omitempty"`
}

func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(configDir, "MachDown")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "config.json"), nil
}

func LoadConfig() (ClientConfig, error) {
	var cfg ClientConfig
	path, err := GetConfigPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GetDefaultConfig(), nil
		}
		return cfg, err
	}

	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func SaveConfig(cfg ClientConfig) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func GetDefaultConfig() ClientConfig {
	home, _ := os.UserHomeDir()
	dlPath := filepath.Join(home, "Downloads", "MachDown")
	
	host, _ := os.Hostname()
	if host == "" {
		host = "machdown-client"
	}

	return ClientConfig{
		ServerURL:    "https://localhost:8888",
		APIKey:       "",
		ClientID:     host,
		DownloadPath: dlPath,
		SiteMappings: map[string]string{
			// Exemplo: "workupload.com": filepath.Join(dlPath, "WorkUpload"),
		},
	}
}
