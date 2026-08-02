# MachDown - Pendências de Desenvolvimento

Este arquivo contém o levantamento das pendências e features ausentes no código atual com base nos documentos SPDD originais (`MACHDOWN-4`, `MACHDOWN-5`, `MACHDOWN-6`). Ele foi elaborado para guiar os próximos passos dos agentes de IA no desenvolvimento e aprimoramento da aplicação.

## 🖥️ 1. Server Core (`server/`)
*   [x] **Deduplicação de Arquivos (ETag):** Implementada em `services/download.go` — após o HEAD request, verifica se já existe um job com o mesmo `ETag` via `repo.FindJobByETag()`. Se sim, retorna o job existente sem duplicar o download. Campo `ETag` adicionado ao modelo `DownloadJob`.
*   [x] **Endpoint de Server-Sent Events (SSE):** Criado `GET /api/downloads/progress` em `api/controllers.go` usando `text/event-stream`. O `ProgressHub` em `services/download.go` gerencia os canais dos clientes conectados e faz broadcast de eventos JSON (`{job_id, status, bytes_done, total_size}`) a cada chunk concluído.
*   [x] **Compressão de Arquivos no Disco:** Middleware `compress.New` do Fiber (backed por `klauspost/compress`) adicionado em `main.go`. Comprime respostas HTTP compressíveis automaticamente (Gzip/Brotli), reduzindo a banda na sincronização com o Client Desktop.
*   [x] **Merkle Tree Completa:** Função `computeMerkleRoot()` em `services/download.go` agrupa os hashes SHA-256 de cada chunk em uma árvore Merkle iterativa e persiste o `RootHash` no banco via `repo.UpdateRootHash()` ao final do job.

## 💻 2. Client Desktop (`client/`)
*   [x] **Validação de Hash (Integridade):** Implementada em `client/sync/daemon.go` — usa `io.TeeReader` para calcular o SHA-256 do arquivo enquanto faz download (sem leitura dupla). Compara com o `root_hash` recebido do servidor. Arquivo é escrito em `.tmp` primeiro e só movido ao destino final após validação bem-sucedida (via `atomicMove`).
*   [x] **Fluxo de Telas (UI):** Implementado em `client/frontend/src/main.js` e `index.html`. Na inicialização, chama `IsConfigured()` (novo método em `app.go`). Se configurado, exibe diretamente o Dashboard com lista de jobs e barra de progresso. Se não, exibe a tela de configuração. Após salvar, transita para o dashboard. Botão de engrenagem (⚙️) no header permite retornar às configurações.

## 🌐 3. Browser Extension (`extension/`)
*   [x] **Fluxo de Telas (UI do Popup):** Implementado em `extension/popup.js` e `popup.html`. No `DOMContentLoaded`, verifica `chrome.storage.sync` — se `api_key`, `server_url` e `client_id` já estiverem salvos, exibe a Tela Principal (status da conexão + dica de uso). Caso contrário, exibe a tela de configuração. Botão ⚙️ no header permite alterar as configurações quando necessário.

---

## 🐛 Bugs Extras Corrigidos (identificados durante implementação)

*   **Bug de Compilação — `controllers.go`:** `json.Marshal` era usado mas `encoding/json` não estava importado, impedindo a compilação do servidor. **Corrigido.**
*   **Bug de Path — `HandleDownloadFile`:** O path do arquivo estava hardcoded como `"downloads/" + jobID`, ignorando o `StoragePath` configurado pelo usuário. Agora usa `downloadService.GetFilePath(jobID)` que delega ao `DiskManager`. **Corrigido.**
*   **Melhoria — Download Atômico no Client:** O client escrevia diretamente no arquivo destino. Se o processo morresse durante o download, ficava um arquivo corrompido no diretório. Agora usa arquivo `.tmp` + `atomicMove` para garantir atomicidade. **Implementado.**

---
**Instruções para o próximo Agente:**
1. Leia o SPDD relevante em `spdd/prompt/` para a task escolhida.
2. Analise o código base respectivo (`server/` ou `client/`).
3. Implemente a funcionalidade e marque o checkbox deste arquivo com um `[x]`.
