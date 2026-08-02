package main

import (
	"log"
	"machdown/server/api"
	"machdown/server/models"
	"machdown/server/repositories"
	"machdown/server/services"
	"machdown/server/storage"
	"os"
	"path/filepath"
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

	db, err := gorm.Open(sqlite.Open(filepath.Join(dbDir, "machdown.db")), &gorm.Config{})
	if err != nil {
		log.Fatal("Falha ao conectar ao banco de dados:", err)
	}

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
		db.Create(&models.APIKey{
			Key:       config.APIKey,
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

	downloadService := services.NewDownloadService(jobRepo, chunkDownloader, diskManager, hub)

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
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-API-Key",
	}))

	// Configurar rotas
	apiGroup := app.Group("/api")
	apiGroup.Use(api.RequireAuth(db, &config))

	apiGroup.Post("/downloads", apiController.HandleEnqueue)
	apiGroup.Get("/downloads/completed", apiController.HandleListCompleted)
	// Endpoint SSE: clientes conectam aqui para receber progresso em tempo real
	apiGroup.Get("/downloads/progress", apiController.HandleSSEProgress)
	apiGroup.Get("/downloads/:id/file", apiController.HandleDownloadFile)
	apiGroup.Post("/downloads/:id/synced", apiController.HandleMarkSynced)

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
	log.Fatal(app.Listen(":" + port))
}
