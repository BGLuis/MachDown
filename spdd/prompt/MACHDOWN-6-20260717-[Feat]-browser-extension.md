# REASONS Canvas: MachDown Browser Extension

## R - Requirements
**Goal**: Construir uma extensão de navegador (Manifest V3) que captura links de download e os envia para o MachDown Server.
**DoD (Definition of Done)**:
- Menu de Contexto (botão direito) "Baixar com MachDown" em links.
- Interceptação de downloads (opcional) ou popup para gerenciar links.
- Popup de Opções (`popup.html`) para configurar a **Server URL**, **API Key** e **Target Client** (ID do PC destino).
- Background Script (Service Worker) que envia a requisição HTTP POST autenticada para o Servidor.
- Notificações de sistema alertando se o envio falhou ou teve sucesso.

## E - Entities
1. `ExtensionConfig`: Dados salvos no Storage (ServerURL, APIKey, TargetClient).
2. `DownloadRequest`: Payload enviado ao servidor (`url`, `target_clients`).

## A - Approach
- **Tecnologia**: Javascript Puro (Vanilla), HTML e CSS. Sem bundlers complexos para facilitar a instalação.
- **Armazenamento**: `chrome.storage.sync` para sincronizar as configurações da extensão na conta do usuário.
- **Comunicação**: API `fetch` padrão para bater no `POST /api/downloads` do Servidor, repassando a `X-API-Key`.

## S - Structure
- `manifest.json`: Arquivo de definição V3.
- `background.js`: Service Worker que escuta cliques no menu de contexto.
- `popup.html` & `popup.js`: Interface gráfica ao clicar no ícone da extensão.
- `icons/`: Pasta com os ícones da extensão.

## O - Operations
1. **Manifest**: Declarar permissões: `contextMenus`, `storage`, `notifications`.
2. **Popup UI**: Criar formulário HTML simples com campos para `Server URL`, `API Key` e `Client ID` + botão de salvar.
3. **Popup Logic**: Implementar leitura e gravação no `chrome.storage`.
4. **Background Logic**: 
   - Ao iniciar (`chrome.runtime.onInstalled`), registrar o Menu de Contexto "Baixar com MachDown".
   - Adicionar listener no Menu de Contexto que pega a URL clicada.
   - Ler `ExtensionConfig` do storage.
   - Fazer o `fetch()` POST pro Servidor com as credenciais.
   - Chamar `chrome.notifications.create` informando o resultado pro usuário.

## N - Norms
- O código deve ser assíncrono (Promises/async-await).
- A interface do popup deve ser pequena (aprox 300x400) e limpa.

## S - Safeguards
- Se o usuário não configurou a API Key ainda, exibir notificação pedindo para clicar na extensão e configurar antes de tentar baixar.
- Tratar falhas de CORS ou rede com notificações claras.
