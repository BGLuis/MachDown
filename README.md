<p align="center">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/header/graph.svg?title=MachDown&subtitle=Acelerador+de+downloads+em+quase+Go+puro&align=left&font=geist-mono&mode=dark" /><img alt="MachDown" src="https://shieldcn.dev/header/graph.svg?title=MachDown&subtitle=Acelerador+de+downloads+em+quase+Go+puro&align=left&font=geist-mono&mode=light" /></picture>
</p>

<p align="center"><b>Português (BR)</b> · <a href="README.en.md">English</a></p>

<p align="center">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/github/stars/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=dark" /><img alt="GitHub Stars" src="https://shieldcn.dev/github/stars/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=light" /></picture>
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/github/forks/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=dark" /><img alt="GitHub Forks" src="https://shieldcn.dev/github/forks/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=light" /></picture>
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/github/watchers/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=dark" /><img alt="Watchers" src="https://shieldcn.dev/github/watchers/BGLuis/MachDown.svg?variant=secondary&size=sm&mode=light" /></picture>
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/github/contributors/BGLuis/MachDown.svg?theme=emerald&size=sm&mode=dark" /><img alt="Contributors" src="https://shieldcn.dev/github/contributors/BGLuis/MachDown.svg?theme=emerald&size=sm&mode=light" /></picture>
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/github/license/BGLuis/MachDown.svg?variant=ghost&size=sm&mode=dark" /><img alt="License" src="https://shieldcn.dev/github/license/BGLuis/MachDown.svg?variant=ghost&size=sm&mode=light" /></picture>
</p>

<p align="center">
  <img alt="Go" src="https://shieldcn.dev/badge/Language-Go-00ADD8.svg?logo=go&variant=branded&size=sm" />
  <img alt="Wails" src="https://shieldcn.dev/badge/Framework-Wails-DF0000.svg?logo=wails&variant=branded&size=sm" />
  <img alt="SQLite" src="https://shieldcn.dev/badge/Database-SQLite-003B57.svg?logo=sqlite&variant=branded&size=sm" />
  <img alt="Docker" src="https://shieldcn.dev/badge/Tool-Docker-2496ED.svg?logo=docker&variant=branded&size=sm" />
  <img alt="JavaScript" src="https://shieldcn.dev/badge/Language-JavaScript-F7DF1E.svg?logo=javascript&variant=branded&size=sm" />
</p>

# 📖 Sobre

O MachDown é um acelerador de downloads dividido em três partes:

- **Servidor** (Go, Fiber e SQLite): recebe a URL, baixa o arquivo em partes (chunks) simultâneas, permite pausar e retomar cada download e informa o progresso em tempo real por SSE. A API exige API Key, as chaves são guardadas com hash Argon2id e o servidor responde por HTTPS com certificado autoassinado.
- **Client desktop** (Wails): sincroniza para a sua máquina os arquivos que o servidor já terminou de baixar e traz um painel para administrar arquivos, chaves de API e configurações do servidor.
- **Extensão para navegadores Chromium** (Manifest V3): intercepta os downloads do navegador, ou envia um link pelo menu "Baixar com MachDown", e repassa a URL ao servidor.

# 📋 Motivo

Queria criar um projeto em quase Go puro e tinha curiosidade para fazer um acelerador de downloads.

# 💻 Como iniciar

### Requisitos

- [Go](https://go.dev/dl/) 1.27.1 (execução local)
- [Make](https://www.gnu.org/software/make/) (execução local)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2.16.0 (execução local, só para o client)
- [Node.js](https://nodejs.org/en/download) com npm (execução local, só para o client)
- [Docker](https://docs.docker.com/get-docker/) com Docker Compose (opção Docker)

### Instalação

1. Clone o repositório:

   ```sh
   git clone https://github.com/BGLuis/MachDown.git
   ```

2. Entre no diretório do projeto:

   ```sh
   cd MachDown
   ```

#### Opção 1: Docker

3. Crie o arquivo `.env` e suba o servidor:

   ```sh
   cp .env.example .env
   docker compose up -d --build
   ```

#### Opção 2: Execução local

3. Inicie o servidor:

   ```sh
   make server
   ```

Na primeira execução, o servidor mostra no log a API Key gerada e o fingerprint SHA-256 do certificado. Use os dois para configurar o client e a extensão.

Com o servidor rodando:

- **Client desktop:** `make client` inicia o Wails em modo de desenvolvimento. Para gerar o executável, use `make build-client`.
- **Extensão:** abra `chrome://extensions`, ative o modo do desenvolvedor, clique em "Carregar sem compactação" e selecione a pasta `extension/`.

# ⚙️ Variáveis de Ambiente

| Variável | Descrição | Padrão |
| :--- | :--- | :--- |
| `MACHDOWN_PORT` | Porta em que o servidor roda | `8888` |
| `MACHDOWN_API_KEY` | API Key fixa para a primeira execução (opcional). Em branco, o servidor gera um UUID | vazio |

# 🤝 Contribuidores

<a href="https://github.com/BGLuis/MachDown/graphs/contributors">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/contributors/BGLuis/MachDown.svg?title=false&preset=transparent&border=false&mode=dark" /><img alt="Contribuidores" src="https://shieldcn.dev/contributors/BGLuis/MachDown.svg?title=false&preset=transparent&border=false&mode=light" /></picture>
</a>

# 📄 Licença

Distribuído sob a licença [MIT](LICENSE).
