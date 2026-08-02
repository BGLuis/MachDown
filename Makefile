.PHONY: server client test build-client build-client-windows build-client-darwin build-server build-all clean

# Executa o servidor Go (Backend)
server:
	cd server && go run main.go

# Executa o client Wails em modo de desenvolvimento (Frontend + Backend local)
client:
	cd client && wails dev

# Roda os testes no backend e no client
test:
	@echo "Rodando testes do servidor..."
	cd server && go test ./...
	@echo "Rodando testes do client..."
	cd client && go test ./...

# Compila o binário do client (Wails) para a plataforma atual (Linux, caso esteja no Linux)
build-client:
	@echo "Compilando o client..."
	cd client && wails build

# Compila o binário do client para Windows (Cross-compilation)
build-client-windows:
	@echo "Compilando o client para Windows..."
	cd client && wails build -platform windows/amd64

# Compila o binário do client para macOS (Cross-compilation)
build-client-darwin:
	@echo "Compilando o client para macOS..."
	cd client && wails build -platform darwin/universal

# Compila o binário do servidor Go
build-server:
	@echo "Compilando o servidor..."
	cd server && go build -o bin/machdown-server main.go

# Compila ambos (Servidor e Client)
build-all: build-server build-client

# Remove os binários gerados
clean:
	@echo "Limpando arquivos de build..."
	rm -rf server/bin
	rm -rf client/build/bin/*
