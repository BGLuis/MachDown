# SPDD Analysis: Client Desktop

## Original Business Requirement
Criar o Client Desktop para o MachDown (acelerador de download self-host).
Requisitos:
1. Sincronização Inteligente ("Rsync" moderno): Conectar ao servidor usando API Key, buscar arquivos com flag `AutoSync` e baixar usando a Merkle Tree para validação de integridade por chunks.
2. Resiliência Total: Se a conexão ou o PC cair, o Client deve usar a Merkle Tree local para retomar exatamente do pedaço que faltou, sem perder progresso.
3. Interface Gráfica (Wails): UI para mostrar a fila do servidor, a fila de downloads locais (com velocidade e ETA) e controles (ex: Limpar disco do servidor).
4. Background Service: Rodar na bandeja do sistema (system tray), enviando notificações nativas de conclusão.
5. Deduplicação e Versionamento: O Client deve baixar os arquivos inteiros; o controle de versão será baseado em arquivos independentes com nomes iterados (ex: arquivo(1).ext) caso haja mudanças na fonte, sem complexidade de diff binário.

## Domain Concept Identification

### Existing Concepts (from codebase)
*(Estrutura de pastas `client` criada. A comunicação com o `server` exigirá structs em `shared`)*

### New Concepts Required
- **LocalSyncEngine**: O núcleo de sincronização no Go. Um loop em background que pergunta à API do servidor se há arquivos prontos com a flag `AutoSync`.
- **ChunkDownloader**: Mecanismo que recebe a estrutura da Merkle Tree do Servidor, inspeciona o arquivo local e pede ao servidor via *HTTP Range Requests* apenas as partes não existentes ou divergentes.
- **WailsApp**: O controlador de ciclo de vida da interface gráfica, unindo eventos do Go com o Frontend.
- **TrayManager**: Serviço que utiliza APIs do sistema operacional (Windows/macOS/Linux) para criar o ícone na bandeja e disparar notificações ("Download Sincronizado").

### Key Business Rules
- O modelo de arquitetura é **Pull**: O servidor nunca tenta se conectar ao client; é o client que verifica a fila no servidor ativamente.
- O Client reutilizará grande parte da lógica matemática da Merkle Tree que o Servidor usou, garantindo simetria (DRY) através da pasta compartilhada `shared/`.
- Falhas locais de chunk (hash inválido) resultam no descarte pontual de bytes no disco local, evitando que todo o arquivo precise ser reiniciado.

## Strategic Approach

### Solution Direction
- **Go + Wails** para gerar um único executável leve e nativo.
- O Frontend usará HTML/JS/CSS moderno (ex: React ou Svelte) com design premium, consumindo eventos enviados pelo backend Go sem bater cabeça com servidores locais.
- A comunicação Go <-> Go (Client para Servidor) será via HTTP REST para facilitar o uso através de proxies e roteadores (já que HTTPS é trivial de configurar com proxies reversos como Nginx ou Caddy).

### Key Design Decisions
- **Complexidade de Versionamento / Diffing Binário:** [O usuário perguntou sobre baixar apenas as alterações daquele arquivo (Diff)] -> [Recomendação: **Rejeitar**. Implementar Delta-encoding e Block-level diffing (como o `rsync` e `Git` fazem internamente) é um pesadelo de engenharia e exige muita CPU, totalmente fora de escopo para um V1. A decisão final é: se a URL for a mesma mas o conteúdo mudou (checando HTTP `ETag`), baixe o novo arquivo completo e salve-o como `nome_do_arquivo(1).ext` para preservar o histórico sem explodir a complexidade do sistema].
- **Modelo de Push/Pull Tempo Real:** [Como saber que o servidor acabou de baixar?] -> [Recomendação: O Client vai manter uma conexão SSE (Server-Sent Events) leve com o Servidor. Assim que o Servidor termina de consolidar um download, ele dispara o evento para o Client que imediatamente inicia a transferência (ChunkDownloader)].
- **Throttle na Interface:** [Atualizações 100x por segundo] -> [Recomendação: O backend limitará o envio de progresso (IPC) ao frontend no máximo 5 vezes por segundo (200ms). Disparar eventos demais trava a interface web no Wails].

### Alternatives Considered
- Usar Fyne (GUI puramente Go). Rejeitado: O usuário foi instruído para usar tecnologias web (Wails) para atingir alto nível estético de interface. Fyne é produtivo, mas a customização gráfica é mais rígida.

## Risk & Gap Analysis

### Requirement Ambiguities
- **Pastas de Destino:** Onde o Client salvará fisicamente as coisas no PC do usuário? (Sugestão: Usar por padrão a pasta `~/Downloads/MachDown` do sistema operacional, mas permitir alterar nas "Configurações" do client, salvas em um arquivo JSON local).

### Edge Cases
- **Múltiplos Clients no mesmo Usuário:** Se eu instalar o Client no meu Desktop e no meu Notebook, ambos usando a mesma API Key. Quando a flag AutoSync for processada por um, ele desmarca a flag do Servidor e o outro não baixa? (Sugestão técnica: A flag no servidor deve registrar um Array de *DeviceIDs* que já sincronizaram, ao invés de um simples booleano `true/false`, ou deixar o usuário configurar o comportamento global).
- **Quedas Bruscas:** Fechar a tampa do notebook ou força tarefa (Task Manager) durante sincronia. O Go deverá escrever o estado atual no disco atômica e constantemente para não corromper a sincronia local.

### Technical Risks
- **CORS no Wails:** Por trás dos panos, o Wails levanta um servidor web interno para renderizar a UI. Fazer requisições REST do frontend (JS) diretamente para o servidor externo MachDown causa bloqueios de CORS. Solução: Todas as chamadas de rede externas DEVEM ser feitas pelo Backend em Go, e o JS apenas invoca os métodos exportados no Wails.

### Acceptance Criteria Coverage
| AC# | Description | Addressable? | Gaps/Notes |
|-----|-------------|--------------|------------|
| 1 | Sync via Merkle Tree | Yes | Depende da arquitetura correta no Servidor. |
| 2 | Retomada | Yes | Chunking HTTP com Range Headers. |
| 3 | Interface Rica | Yes | Wails facilita isso. |
| 4 | Tray e Notificações | Yes | Bibliotecas de terceiros (`wailsapp/mimetype` ou `getlantern/systray`). |
| 5 | Deduplicação sem Diff | Yes | Muito mais simples e robusto. |
