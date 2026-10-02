package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"machdown/server/api"
	"machdown/server/models"
	"machdown/server/repositories"
	"machdown/server/services"
	"machdown/server/storage"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func main() {
	// 1. Inicializar banco SQLite
	dbDir := "./data"
	os.MkdirAll(dbDir, 0755)

	certFile := filepath.Join(dbDir, "server.crt")
	keyFile := filepath.Join(dbDir, "server.key")
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		log.Println("Gerando certificados TLS self-signed...")
		if err := generateSelfSignedCert(certFile, keyFile); err != nil {
			log.Fatal("Falha ao gerar certificados TLS:", err)
		}
	}

	db, err := gorm.Open(sqlite.Open(filepath.Join(dbDir, "machdown.db")), &gorm.Config{})
	if err != nil {
		log.Fatal("Falha ao conectar ao banco de dados:", err)
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")
	db.Exec("PRAGMA cache_size=-64000")
	db.Exec("PRAGMA busy_timeout=5000")

	// Auto Migrate (inclui o novo campo ETag, APIKey e Client)
	db.AutoMigrate(&models.DownloadJob{}, &models.ChunkTask{}, &models.ServerConfig{}, &models.APIKey{}, &models.Client{})

	// 2. Carregar ou criar configuração
	var config models.ServerConfig
	if db.First(&config).Error != nil {
		initialAPIKey := os.Getenv("MACHDOWN_API_KEY")
		if initialAPIKey == "" {
			initialAPIKey = uuid.New().String()
		}

		config = models.ServerConfig{
			APIKey:                 initialAPIKey,
			StoragePath:            "./downloads",
			MaxConcurrentDownloads: 8,
		}
		db.Create(&config)
		log.Printf("\n=========================================\n")
		log.Printf("PRIMEIRA EXECUÇÃO DETECTADA!\n")
		log.Printf("Sua MachDown API Key é: %s\n", config.APIKey)
		log.Printf("Copie essa chave para usar na Extension e no Client.\n")
		log.Printf("=========================================\n\n")
	} else {
		log.Println("Servidor inicializado com configuração existente.")
	}

	// Migração para a tabela APIKey
	var keyCount int64
	db.Model(&models.APIKey{}).Count(&keyCount)
	if keyCount == 0 && config.APIKey != "" {
		id, keyHash, prefix := models.GenerateAPIKeyData(config.APIKey)
		db.Create(&models.APIKey{
			ID:        id,
			KeyHash:   keyHash,
			KeyPrefix: prefix,
			Name:      "Chave Inicial",
			CreatedAt: time.Now(),
		})
	}

	// 3. Inicializar camadas
	jobRepo := repositories.NewJobRepository(db)
	diskManager := storage.NewDiskManager(config.StoragePath)
	chunkDownloader := services.NewChunkDownloader(diskManager)

	// ProgressHub: gerencia os canais SSE dos clientes conectados
	hub := services.NewProgressHub()

	downloadService := services.NewDownloadService(jobRepo, chunkDownloader, diskManager, hub, config.MaxConcurrentDownloads)

	apiController := api.NewAPIController(downloadService)

	// 4. Configurar servidor HTTP (Fiber)
	app := fiber.New(fiber.Config{
		DisableKeepalive: false,
	})

	// Compressão automática de respostas compressíveis (Gzip/Brotli via klauspost/compress)
	// Isso reduz a banda ao servir arquivos para o client desktop.
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			if origin == "" || origin == "null" {
				return true
			}
			if strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "https://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://127.0.0.1") ||
				strings.HasPrefix(origin, "chrome-extension://") ||
				strings.HasPrefix(origin, "wails://") {
				return true
			}
			allowed := os.Getenv("MACHDOWN_ALLOWED_ORIGINS")
			if allowed != "" {
				for _, o := range strings.Split(allowed, ",") {
					if strings.TrimSpace(o) == origin {
						return true
					}
				}
			}
			return false
		},
		AllowHeaders: "Origin, Content-Type, Accept, X-API-Key, X-Client-ID",
	}))

	// Configurar rotas
	apiGroup := app.Group("/api")
	apiGroup.Use(api.RateLimitMiddleware())
	apiGroup.Use(api.RequireAuth(db, &config))

	apiGroup.Post("/downloads", apiController.HandleEnqueue)
	apiGroup.Get("/downloads/completed", apiController.HandleListCompleted)
	// Endpoint SSE: clientes conectam aqui para receber progresso em tempo real
	apiGroup.Get("/downloads/progress", apiController.HandleSSEProgress)
	apiGroup.Get("/downloads/:id/file", apiController.HandleDownloadFile)
	apiGroup.Post("/downloads/:id/synced", apiController.HandleMarkSynced)
	apiGroup.Post("/jobs/:id/pause", apiController.HandlePauseDownload)
	apiGroup.Post("/jobs/:id/resume", apiController.HandleResumeDownload)
	apiGroup.Get("/events", apiController.HandleEvents)

	adminController := api.NewAdminController(db, diskManager)
	apiGroup.Get("/admin/jobs", adminController.HandleListAllJobs)
	apiGroup.Get("/admin/files", adminController.HandleListFiles)
	apiGroup.Delete("/admin/files/:filename", adminController.HandleDeleteFile)
	apiGroup.Post("/admin/files/delete-batch", adminController.HandleDeleteBatch)
	apiGroup.Post("/admin/files/sync-batch", adminController.HandleForceSync)
	apiGroup.Get("/admin/config", adminController.HandleGetConfig)
	apiGroup.Put("/admin/config", adminController.HandleUpdateConfig)
	apiGroup.Post("/admin/clean", adminController.HandleClean)
	
	apiGroup.Get("/admin/apikeys", adminController.HandleListAPIKeys)
	apiGroup.Post("/admin/apikeys", adminController.HandleCreateAPIKey)
	apiGroup.Delete("/admin/apikeys/:key", adminController.HandleDeleteAPIKey)
	apiGroup.Get("/admin/clients", adminController.HandleListClients)

	// Iniciar servidor
	port := os.Getenv("MACHDOWN_PORT")
	if port == "" {
		port = "8888"
	}
	
	log.Printf("MachDown Server rodando em :%s\n", port)
	
	go func() {
		if err := app.ListenTLS(":"+port, certFile, keyFile); err != nil {
			log.Printf("Erro no servidor: %v\n", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Encerrando servidor graciosamente...")
	if err := app.ShutdownWithTimeout(30 * time.Second); err != nil {
		log.Printf("Erro ao encerrar servidor: %v\n", err)
	}
	log.Println("Servidor encerrado")
}

func generateSelfSignedCert(certFile, keyFile string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"MachDown"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))
	template.DNSNames = append(template.DNSNames, "localhost")

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}

	return nil
}
