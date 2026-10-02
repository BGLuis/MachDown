<p align="center">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/header/graph.svg?title=MachDown&subtitle=Download+accelerator+in+almost+pure+Go&align=left&font=geist-mono&mode=dark" /><img alt="MachDown" src="https://shieldcn.dev/header/graph.svg?title=MachDown&subtitle=Download+accelerator+in+almost+pure+Go&align=left&font=geist-mono&mode=light" /></picture>
</p>

<p align="center"><b>English</b> · <a href="README.md">Português (BR)</a></p>

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

# 📖 About

MachDown is a download accelerator split into three parts:

- **Server** (Go, Fiber and SQLite): receives the URL, downloads the file in concurrent parts (chunks), lets you pause and resume each download, and reports progress in real time over SSE. The API requires an API Key, keys are stored as Argon2id hashes, and the server answers over HTTPS with a self-signed certificate.
- **Desktop client** (Wails): syncs to your machine the files the server has already finished downloading, and includes a panel to manage files, API keys and server settings.
- **Chromium browser extension** (Manifest V3): intercepts the browser's downloads, or sends a link through the "Baixar com MachDown" context menu, and forwards the URL to the server.

# 📋 Motivation

I wanted to build a project in almost pure Go, and I was curious about making a download accelerator.

# 💻 Getting Started

### Requirements

- [Go](https://go.dev/dl/) 1.27.1 (local execution)
- [Make](https://www.gnu.org/software/make/) (local execution)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2.16.0 (local execution, client only)
- [Node.js](https://nodejs.org/en/download) with npm (local execution, client only)
- [Docker](https://docs.docker.com/get-docker/) with Docker Compose (Docker option)

### Installation

1. Clone the repository:

   ```sh
   git clone https://github.com/BGLuis/MachDown.git
   ```

2. Enter the project directory:

   ```sh
   cd MachDown
   ```

#### Option 1: Docker

3. Create the `.env` file and start the server:

   ```sh
   cp .env.example .env
   docker compose up -d --build
   ```

#### Option 2: Local execution

3. Start the server:

   ```sh
   make server
   ```

On the first run, the server logs the generated API Key and the SHA-256 fingerprint of its certificate. Use both to configure the client and the extension.

With the server running:

- **Desktop client:** `make client` starts Wails in development mode. To build the executable, use `make build-client`.
- **Extension:** open `chrome://extensions`, enable developer mode, click "Load unpacked" and select the `extension/` folder.

# ⚙️ Environment Variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `MACHDOWN_PORT` | Port the server listens on | `8888` |
| `MACHDOWN_API_KEY` | Fixed API Key for the first run (optional). When empty, the server generates a UUID | empty |

# 🤝 Contributors

<a href="https://github.com/BGLuis/MachDown/graphs/contributors">
  <picture><source media="(prefers-color-scheme: dark)" srcset="https://shieldcn.dev/contributors/BGLuis/MachDown.svg?title=false&preset=transparent&border=false&mode=dark" /><img alt="Contributors" src="https://shieldcn.dev/contributors/BGLuis/MachDown.svg?title=false&preset=transparent&border=false&mode=light" /></picture>
</a>

# 📄 License

Distributed under the [MIT](LICENSE) license.
