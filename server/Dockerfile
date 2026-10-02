FROM golang:1.27-alpine AS builder

WORKDIR /app

# Adicionar git caso alguma dependência precise
RUN apk add --no-cache git

# Fazer o download das dependências Go
COPY go.mod go.sum ./
RUN go mod download

# Copiar o resto do código fonte
COPY . .

# Compilar o binário como estático
RUN CGO_ENABLED=0 GOOS=linux go build -o machdown-server main.go

# ---
# Etapa Final (Imagem menor de produção)
# ---
FROM alpine:latest
WORKDIR /app

# Instalar certificados CA para chamadas HTTPS (caso o servidor faça downloads na internet)
RUN apk add --no-cache ca-certificates

# Criar as pastas esperadas pelo main.go antes de rodar
RUN mkdir -p /app/data /app/downloads

# Copiar apenas o binário compilado da etapa anterior
COPY --from=builder /app/machdown-server .

# Expor a porta 8888 (conforme log/Listen no main.go)
EXPOSE 8888

# Executar a aplicação
CMD ["./machdown-server"]
