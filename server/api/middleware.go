package api

import (
	"machdown/server/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RequireAuth is a middleware that checks the X-API-Key header against the APIKey table
func RequireAuth(db *gorm.DB, config *models.ServerConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Missing X-API-Key header",
			})
		}
		
		var key models.APIKey
		if err := db.Where("key = ?", apiKey).First(&key).Error; err != nil {
			// Fallback para master key do config, se a tabela não tiver
			if apiKey != config.APIKey {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "Unauthorized: Invalid X-API-Key",
				})
			}
		}
		
		// Atualizar tabela de clientes se enviar client_id
		clientID := c.Get("X-Client-ID")
		if clientID == "" {
			clientID = c.Query("client_id")
		}

		if clientID != "" {
			db.Save(&models.Client{
				ID:       clientID,
				LastSeen: time.Now(),
				IP:       c.IP(),
			})
		}

		return c.Next()
	}
}
