package main

import (
	"context"
	"machdown-client/config"
	"machdown-client/sync"
)

// App struct
type App struct {
	ctx    context.Context
	daemon *sync.SyncDaemon
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cfg, _ := config.LoadConfig()
	a.daemon = sync.NewSyncDaemon(cfg)
	a.daemon.Start()
}

func (a *App) shutdown(ctx context.Context) {
	if a.daemon != nil {
		a.daemon.Stop()
	}
}

// GetConfig returns the current client configuration
func (a *App) GetConfig() config.ClientConfig {
	cfg, _ := config.LoadConfig()
	return cfg
}

// SaveConfig saves the client configuration and updates the daemon
func (a *App) SaveConfig(cfg config.ClientConfig) error {
	err := config.SaveConfig(cfg)
	if err == nil && a.daemon != nil {
		a.daemon.UpdateConfig(cfg)
	}
	return err
}

// GetJobs returns the active sync jobs
func (a *App) GetJobs() []sync.SyncJob {
	if a.daemon != nil {
		return a.daemon.GetJobs()
	}
	return []sync.SyncJob{}
}

// IsConfigured retorna true se o usuário já preencheu os dados obrigatórios de configuração.
// O frontend usa isso para decidir entre exibir a tela de configuração ou o dashboard.
func (a *App) IsConfigured() bool {
	cfg, err := config.LoadConfig()
	if err != nil {
		return false
	}
	return cfg.APIKey != "" && cfg.ServerURL != "" && cfg.ClientID != ""
}

