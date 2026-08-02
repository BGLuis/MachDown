package api

import (
	"encoding/json"
	"fmt"
	"machdown/server/models"
	"machdown/server/storage"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type AdminController struct {
	db          *gorm.DB
	diskManager *storage.DiskManager
}

func NewAdminController(db *gorm.DB, diskManager *storage.DiskManager) *AdminController {
	return &AdminController{db: db, diskManager: diskManager}
}

type FileInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Status string `json:"status"`
}

// HandleListFiles lista os arquivos armazenados no servidor
func (ctrl *AdminController) HandleListFiles(c *fiber.Ctx) error {
	var config models.ServerConfig
	ctrl.db.First(&config)

	files, err := os.ReadDir(config.StoragePath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var fileList []FileInfo
	for _, f := range files {
		if !f.IsDir() {
			info, err := f.Info()
			if err == nil {
				var job models.DownloadJob
				if err := ctrl.db.Where("id = ?", f.Name()).First(&job).Error; err == nil {
					fileList = append(fileList, FileInfo{
						ID:     f.Name(),
						Name:   job.FileName,
						Size:   info.Size(),
						Status: job.Status,
					})
				} else {
					fileList = append(fileList, FileInfo{
						ID:     f.Name(),
						Name:   "Órfão: " + f.Name(),
						Size:   info.Size(),
						Status: "Unknown",
					})
				}
			}
		}
	}

	return c.JSON(fileList)
}

// HandleDeleteFile exclui um arquivo do storage e o registro do db se existir
func (ctrl *AdminController) HandleDeleteFile(c *fiber.Ctx) error {
	filename := c.Params("filename")

	var config models.ServerConfig
	ctrl.db.First(&config)

	filePath := filepath.Join(config.StoragePath, filename)

	// Validação simples de path traversal
	if filepath.Dir(filePath) != config.StoragePath && filepath.Dir(filePath) != filepath.Clean(config.StoragePath) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid filename"})
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Exclui job associado
	ctrl.db.Where("id = ?", filename).Delete(&models.DownloadJob{})
	ctrl.db.Where("job_id = ?", filename).Delete(&models.ChunkTask{})

	return c.SendStatus(fiber.StatusOK)
}

// HandleGetConfig retorna a configuração do servidor
func (ctrl *AdminController) HandleGetConfig(c *fiber.Ctx) error {
	var config models.ServerConfig
	ctrl.db.First(&config)
	return c.JSON(config)
}

// HandleUpdateConfig atualiza a configuração do servidor
func (ctrl *AdminController) HandleUpdateConfig(c *fiber.Ctx) error {
	var config models.ServerConfig
	ctrl.db.First(&config)

	var updateData struct {
		APIKey                 string `json:"api_key"`
		StoragePath            string `json:"storage_path"`
		MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
	}

	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if updateData.APIKey != "" {
		config.APIKey = updateData.APIKey
	}
	if updateData.MaxConcurrentDownloads > 0 {
		config.MaxConcurrentDownloads = updateData.MaxConcurrentDownloads
	}

	ctrl.db.Save(&config)

	return c.JSON(config)
}

// HandleClean limpa jobs com erro e seus arquivos órfãos
func (ctrl *AdminController) HandleClean(c *fiber.Ctx) error {
	var config models.ServerConfig
	ctrl.db.First(&config)

	var jobs []models.DownloadJob
	ctrl.db.Where("status = ?", "Error").Find(&jobs)

	count := 0
	for _, j := range jobs {
		os.Remove(filepath.Join(config.StoragePath, j.ID))
		ctrl.db.Where("job_id = ?", j.ID).Delete(&models.ChunkTask{})
		ctrl.db.Delete(&j)
		count++
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Cleaned %d error jobs", count)})
}

// HandleDeleteBatch exclui múltiplos arquivos
func (ctrl *AdminController) HandleDeleteBatch(c *fiber.Ctx) error {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	var config models.ServerConfig
	ctrl.db.First(&config)

	for _, id := range req.IDs {
		filePath := filepath.Join(config.StoragePath, id)
		// Basic path traversal check
		if filepath.Dir(filePath) == config.StoragePath || filepath.Dir(filePath) == filepath.Clean(config.StoragePath) {
			os.Remove(filePath)
			ctrl.db.Where("id = ?", id).Delete(&models.DownloadJob{})
			ctrl.db.Where("job_id = ?", id).Delete(&models.ChunkTask{})
		}
	}
	return c.SendStatus(fiber.StatusOK)
}

// HandleForceSync força a sincronização de arquivos para um client específico
func (ctrl *AdminController) HandleForceSync(c *fiber.Ctx) error {
	var req struct {
		IDs      []string `json:"ids"`
		ClientID string   `json:"client_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}
	if req.ClientID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client_id is required"})
	}

	for _, id := range req.IDs {
		var job models.DownloadJob
		if err := ctrl.db.Where("id = ?", id).First(&job).Error; err == nil {
			var clients []string
			if job.TargetClients != "" {
				json.Unmarshal([]byte(job.TargetClients), &clients)
			}
			
			// Add if not present
			found := false
			for _, client := range clients {
				if client == req.ClientID {
					found = true
					break
				}
			}
			if !found {
				clients = append(clients, req.ClientID)
				newClientsJSON, _ := json.Marshal(clients)
				job.TargetClients = string(newClientsJSON)
				ctrl.db.Save(&job)
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleListAPIKeys lista as chaves de API do servidor
func (ctrl *AdminController) HandleListAPIKeys(c *fiber.Ctx) error {
	var keys []models.APIKey
	if err := ctrl.db.Find(&keys).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(keys)
}

// HandleCreateAPIKey cria uma nova chave de API
func (ctrl *AdminController) HandleCreateAPIKey(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
		Key  string `json:"key"` // Se vazio, não cria. Cliente deve enviar.
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if req.Name == "" || req.Key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name and key are required"})
	}

	newKey := models.APIKey{
		Key:       req.Key,
		Name:      req.Name,
		// time.Now() requires "time" import, but we didn't import it in admin.go! 
		// Actually, let's just let Gorm handle CreatedAt by using a pointer or just not setting it if we don't have time imported.
		// Wait, I can just not set CreatedAt and Gorm might auto-set it, or I can import time.
		// Let me just not set it, or update imports. 
		// Actually, let's use the DB's current time via GORM.
	}

	if err := ctrl.db.Create(&newKey).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newKey)
}

// HandleDeleteAPIKey exclui uma chave de API
func (ctrl *AdminController) HandleDeleteAPIKey(c *fiber.Ctx) error {
	key := c.Params("key")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "key param is required"})
	}

	var keyCount int64
	ctrl.db.Model(&models.APIKey{}).Count(&keyCount)
	if keyCount <= 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete the last API key"})
	}

	if err := ctrl.db.Where("key = ?", key).Delete(&models.APIKey{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleListClients lista todos os clientes registrados
func (ctrl *AdminController) HandleListClients(c *fiber.Ctx) error {
	var clients []models.Client
	if err := ctrl.db.Order("last_seen desc").Find(&clients).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(clients)
}

// HandleListAllJobs lista todos os jobs do servidor
func (ctrl *AdminController) HandleListAllJobs(c *fiber.Ctx) error {
	var jobs []models.DownloadJob
	if err := ctrl.db.Order("created_at desc").Limit(50).Find(&jobs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(jobs)
}
