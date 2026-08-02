# SPDD Analysis: Browser Extension

## Original Business Requirement
Criar a Extensão de Navegador para o MachDown (acelerador de download self-host).
Requisitos:
1. Menu de Contexto (Right-click): Ao clicar com o botão direito sobre um link, exibir a opção 'Baixar com MachDown' e 'Baixar normalmente pelo navegador'.
2. Interceptação Automática (configurável): Interceptar downloads do navegador e enviá-los ao servidor.
3. Extração de Dados: Capturar e enviar URL, Cookies da página e User-Agent para o servidor.
4. Tela de Configurações: Página para configurar URL do Servidor MachDown e API Key, além de ativar/desativar a interceptação.
5. Feedback e Progresso Visual: A extensão deve mostrar o que está sendo baixado (progresso).
6. Ação Pós-Download: Durante o download no servidor, o usuário pode selecionar na extensão que, quando terminar, o arquivo deve ser baixado automaticamente para o computador dele (via Client).

## Domain Concept Identification

### Existing Concepts (from codebase)
*(Projeto novo, sem conceitos existentes na base de código. Estrutura de pastas `server`, `client`, `extension` criada)*

### New Concepts Required
- **DownloadRequest**: O objeto de dados que a extensão envia para o servidor (contendo URL, Cookies, UserAgent).
- **DownloadTask**: A representação de um download ativo no servidor, que a extensão precisará consultar para exibir o progresso no popup.
- **ServerConfiguration**: A configuração local salva no storage do navegador (URL do Servidor, API Key, modo de interceptação).
- **AutoSyncFlag**: Um sinalizador gerado na extensão (e enviado para atualizar uma `DownloadTask` no servidor) para que o *Client* desktop faça o pull imediato do arquivo quando ele for finalizado no servidor.

### Key Business Rules
- A extensão não executa o download do arquivo em si; ela atua como um injetor/delegação de requisições para o Servidor MachDown.
- Autenticação via `API Key` é estritamente obrigatória na comunicação com o Servidor.
- Os Cookies da sessão do site de origem e o User-Agent devem ser perfeitamente espelhados no Payload enviado ao servidor para contornar proteções, logins e captchas.
- A sincronização para a máquina final depende do 'Client', mas a extensão funciona como interface de comando para acionar esse fluxo (`AutoSyncFlag`).

## Strategic Approach

### Solution Direction
- Construir a extensão usando o padrão **Manifest V3** (WebExtensions API), garantindo compatibilidade multiplataforma (Chrome, Firefox, Edge, Brave, etc).
- Utilizar `Service Workers` (Background Scripts) para gerenciar a injeção do menu de contexto, interceptação da API `chrome.downloads` e comunicação RESTful com o Servidor MachDown.
- Para mostrar o progresso em tempo real, a UI da extensão (Popup) consumirá os dados do servidor. Devido a limitações de vida útil do Manifest V3, WebSockets são viáveis no popup aberto, ou apenas um leve Polling (HTTP GET) enquanto o usuário estiver com o popup focado.

### Key Design Decisions
- **Uso do Manifest V3:** [Arquitetura imposta por motores Chromium] -> [Recomendação: Implementar lógica stateless no Service Worker, salvando todas as configurações de host e token no `chrome.storage.sync`].
- **Mecânica de Interceptação:** [Usar `chrome.downloads` vs injetar scripts no DOM] -> [Recomendação: Usar a API `chrome.downloads.onCreated`. Quando o navegador tentar baixar, a extensão pega a URL, cancela o download original nativo e envia os dados para o servidor. Injetar no DOM quebra frequentemente e não cobre todos os cenários].
- **O desafio dos Cookies:** [Como enviar cookies de terceiros] -> [Recomendação: A extensão precisará de permissão host (ex: `<all_urls>`) e da permissão `cookies` para extrair os cookies específicos da URL fonte do download antes de despachar o request].

### Alternatives Considered
- *Manifest V2*: Rejeitado por estar sendo descontinuado ativamente pelas lojas de extensões.

## Risk & Gap Analysis

### Requirement Ambiguities
- **"Baixar normalmente pelo navegador":** Se a interceptação automática estiver LIGADA globalmente, como o botão "Baixar normalmente" (no click direito) avisa a extensão para NÃO interceptar este download específico? (Sugestão: Criar um mecanismo de "state temporário" ou whitelist momentânea antes de disparar o clique na aba).

### Edge Cases
- **Downloads Locais (Blobs/Data URIs):** Downloads gerados dinamicamente via Javascript (ex: `blob:http://...` ou `data:image/...`). O servidor MachDown não tem como baixá-los, pois não são URLs acessíveis externamente. A extensão precisará validar a URL e ignorar a interceptação caso sejam arquivos gerados estritamente no cliente.
- **Redes Locais e HTTPS:** Se o Servidor MachDown estiver rodando em uma rede local com certificado self-signed ou sem HTTPS (ex: HTTP, localhost), navegadores modernos podem bloquear a extensão de fazer chamadas POST para ele (Mixed Content). O servidor precisará de suporte a CORS adequado, ou rodar localmente sem travas de Mixed Content.

### Technical Risks
- **Limites do Background Script (V3):** Service workers adormecem após alguns segundos. Qualquer requisição pesada para o servidor MachDown não pode travar ou exceder o tempo, caso contrário a extensão morre no meio do processo e perde o download.
- **CORS e Header Injections:** Cuidado redobrado ao montar os headers da requisição, certificando que o Servidor permita requisições externas através dos headers `Access-Control-Allow-Origin`.

### Acceptance Criteria Coverage
| AC# | Description | Addressable? | Gaps/Notes |
|-----|-------------|--------------|------------|
| 1 | Menu de Contexto | Yes | Bypass de interceptação exigirá lógica especial para ignorar temporariamente a trigger. |
| 2 | Interceptação Automática | Yes | Adicionar filtro contra `blob:` e `data:`. |
| 3 | Captura de Dados/Cookies | Yes | Exige fortes permissões no arquivo `manifest.json`. |
| 4 | Opções / Settings | Yes | Página HTML estática conectada ao `chrome.storage`. |
| 5 | Progresso Visual | Yes | Pode exigir endpoint específico no servidor (ex: `/api/downloads`). |
| 6 | Ação AutoSync | Yes | Requer um endpoint de Update (ex: `PUT /api/downloads/{id}`) no Servidor, sinalizando a intenção. |
