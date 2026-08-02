# SPDD Analysis: Server Core

## Original Business Requirement
Criar o Servidor do MachDown (acelerador de download self-host).
Requisitos:
1. API REST de Entrada: Receber links da extensão (com URL, Cookies, User-Agent, AutoSync). Controle de pausar/retomar/cancelar.
2. Motor de Aceleração (Download Engine): Baixar arquivos dividindo-os em chunks via HTTP Range Requests usando goroutines. Retomar downloads em caso de falhas. 
3. Verificação de Integridade (Merkle Tree): Utilizar uma árvore de Merkle (Merkle tree) para criar hashes dos chunks, verificando se foram baixados e unidos corretamente. Essa mesma árvore servirá depois para o Client verificar a integridade ao baixar.
4. Banco de Dados: SQLite para gerenciar a fila e metadados.
5. Autenticação: API Key na comunicação.
6. Tempo Real: WebSockets/SSE para reportar progresso.
7. Gerenciamento de Disco: Permitir que o usuário escolha quando limpar o disco (arquivos não devem ser apagados automaticamente sem permissão).
8. Transferência para o Client: Avaliar compressão dos dados durante a transferência pela rede do Servidor para o Client. (Criptografia de ponta-a-ponta ficará para o futuro).

## Domain Concept Identification

### Existing Concepts (from codebase)
*(Estrutura de pastas `server` criada, porém base de código Go ainda não inicializada).*

### New Concepts Required
- **DownloadJob**: Entidade principal salva no DB. Representa o arquivo como um todo, seu status (Baixando, Pausado, Completo), o caminho no disco e a raiz (Root Hash) da Merkle Tree.
- **ChunkTask**: Representação em memória e no banco de cada pedaço (ex: partes de 4MB) de um arquivo em download.
- **MerkleVerifier**: Componente focado em gerar hashes (ex: SHA-256) dos chunks *on-the-fly* e construir a estrutura da árvore.
- **DiskManager**: Serviço para alocar o arquivo no disco, lidar com a junção das partes, e processar os comandos de "Limpeza Manual" enviados pelo usuário.

### Key Business Rules
- Os downloads são processos independentes (Goroutines) da requisição da API; a API enfileira o trabalho e retorna rapidamente.
- **Integridade é rei:** Se um Chunk baixar mas não bater com o Hash experado na verificação da Merkle Tree, ele é invalidado e baixado novamente, sem perder o arquivo inteiro.
- Os arquivos ficam retidos no disco do Servidor até uma instrução explícita do usuário (via dashboard ou comandos da extensão/client) para excluí-los (regra de retenção manual).
- Arquivos em trânsito do Servidor para o Client usarão compressão HTTP nativa sempre que possível.

## Strategic Approach

### Solution Direction
- A stack do servidor será puramente em **Go**, extraindo o máximo de I/O de rede e disco.
- Para gerenciar conexões concorrentes, usaremos um sistema de *Worker Pool* em Go para não estourar os limites de porta TCP do host ao baixar múltiplos arquivos fragmentados.
- O SQLite funcionará como fonte da verdade (Source of Truth). Usaremos a lib `modernc.org/sqlite` que dispensa CGO, garantindo que a build rode em qualquer plataforma sem instalar compiladores C.
- A **Merkle Tree** funcionará assim: cada Chunk de X megabytes gera um Node. O servidor junta os hashes para formar os galhos até a Raiz. Essa árvore pode ser serializada (JSON) e devolvida ao Client, facilitando a vida do Client para saber se alguma parte corrompeu durante o trânsito da rede interna.

### Key Design Decisions
- **Merkle Tree & Chunks:** [Qual o tamanho do bloco?] -> [Recomendação: Tamanho fixo de 2MB a 8MB por chunk. Tamanhos fixos deixam o cálculo da Merkle Tree muito mais rápido. O hash será SHA-256 pela segurança sem grande perda de velocidade].
- **Compressão de Rede vs Tipos de Arquivo:** [Fazer o gzip do arquivo] -> [Recomendação: Ativar middlewares Gzip/Brotli (nível HTTP) na API Go. Porém, arquivos como `.zip`, `.mp4` ou `.mkv` não se beneficiam de compressão extra. O ganho da compressão será enorme em arquivos texto, iso esparsos, etc.].
- **Comunicação Tempo Real:** [WebSockets vs SSE] -> [Recomendação: SSE (Server-Sent Events) é muito mais fácil de implementar para fluxos de dados unidirecionais (Servidor -> Cliente). WebSockets seriam over-engineering para apenas reportar `%` de download].

### Alternatives Considered
- Usar apenas verificação final (MD5/SHA1 do arquivo inteiro após o download). Rejeitado: O usuário especificou que quer a Merkle Tree, o que é muito melhor para baixar arquivos de 100GB, permitindo reparar partes minúsculas ao invés de perder tudo.

## Risk & Gap Analysis

### Requirement Ambiguities
- **Comando de Limpar o Disco:** Como o usuário não quer exclusão automática, onde ele vai clicar para "Limpar"? (Sugestão: Adicionar um botão no Client e nas Opções da Extensão "Liberar Espaço no Servidor" que chama um endpoint no Go, além de permitir limpar itens específicos).

### Edge Cases
- **Crash do Servidor (Rebooting):** Se a energia cair, o banco de dados pode estar marcando um arquivo como "Baixando". Na inicialização, o Go deve varrer os jobs inacabados, validar no disco o que já existe via Merkle Tree, e reiniciar apenas as partes faltantes.
- **Server Rate Limiting (Bans):** Alguns servidores bloqueiam IPs que fazem muitas requisições "Range" paralelas. Será preciso um limitador de concorrência global (ex: MaxConnectionsPerHost).

### Technical Risks
- **Cálculo da Merkle Tree:** Hashear gigabytes gasta CPU. Solução técnica é usar `io.TeeReader` para calcular o Hash enquanto a rede grava pro disco (stream pass-through), evitando ler do disco duas vezes.
- **Espaço em Disco Crítico:** O servidor deve pausar os downloads e avisar se o HD chegar, digamos, a 95% de uso, para evitar corrupção do SQLite.

### Acceptance Criteria Coverage
| AC# | Description | Addressable? | Gaps/Notes |
|-----|-------------|--------------|------------|
| 1 | API e Motor de Downloads | Yes | Gerência de goroutines / WaitGroups. |
| 2 | Merkle Tree de Integridade | Yes | Peça fundamental do sistema; precisará de uma package própria. |
| 3 | SQLite DB | Yes | Fila e metadados persistentes. |
| 4 | Sem Apagar Sozinho (Retenção) | Yes | Fácil, endpoint de Delete explícito. |
| 5 | Compressão HTTP | Yes | Middleware padrão no router Go (ex: Fiber ou Echo). |
