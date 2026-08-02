# REASONS Canvas: MachDown Client Desktop

## R - Requirements
**Goal**: Build a desktop client that synchronizes downloaded files from the Server to the user's machine, offering a GUI to configure the download path and view synced files.
**DoD (Definition of Done)**:
- Client starts a background daemon (Go) to periodically poll the Server for completed downloads.
- Downloaded files are assembled/verified locally using Merkle Tree hashes (ou apenas Hash simples na v1).
- GUI (via Wails) permite configurar Server URL, API Key, Client ID (nome do PC) e Local Download Path.
- Configurações são salvas localmente num arquivo `config.json`.

## E - Entities
1. `ClientConfig`: Struct (ServerURL, APIKey, ClientID, DownloadPath).
2. `SyncJob`: Struct de progresso (JobID, FileName, TotalSize, Downloaded, Status).

## A - Approach
- **Framework**: Wails (Go para o backend, HTML/JS/CSS puro para o Frontend para simplificar).
- **Sync Strategy**: Modelo Pull. O Go backend roda um `time.Ticker` (ex: a cada 10s) fazendo um `GET /api/sync/pending?client_id=X` no Servidor.
- **Armazenamento**: O arquivo baixa para uma pasta `.tmp` e só é movido para a pasta final configurada após a validação.

## S - Structure
- `frontend/`: Código da UI (HTML/JS/CSS).
- `app.go`: Wails Application struct, contendo os métodos exportados para o Frontend (bindings).
- `config/manager.go`: Responsável por ler e gravar o `config.json` na pasta do usuário.
- `sync/daemon.go`: O Worker em background que se comunica com a API do Servidor e gerencia os downloads locais.

## O - Operations
1. **ConfigManager**: Implementar `LoadConfig() -> ClientConfig` e `SaveConfig(ClientConfig) -> error`.
2. **App Bindings**: Criar métodos no `app.go` (`GetConfig`, `SaveConfig`, `GetSyncStatus`) para a interface JS chamar.
3. **SyncDaemon**: Implementar `Start()` que inicia a rotina de polling baseada na configuração atual.
4. **SyncWorker**: Implementar `fetchPendingJobs()` e `downloadFile(jobID, hash)` usando o cliente HTTP padrão do Go.

## N - Norms
- Usar `os.UserConfigDir()` para salvar o `config.json` de forma correta dependendo do SO (Windows/Linux/Mac).
- Usar `os.UserHomeDir() + /Downloads/MachDown` como diretório de download padrão.
- O Frontend só se comunica com o backend via Wails Bindings.

## S - Safeguards
- O `SyncDaemon` deve pausar e aguardar caso a `API Key` ou `Server URL` estejam vazios nas configurações.
- Nunca sobrescrever um arquivo existente na pasta de downloads sem verificar se ele já foi completamente baixado (Deduplicação local).
