package api

import (
	"crypto/sha256"
	"encoding/hex"
	"machdown/server/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"gorm.io/gorm"
)

const (
	rateLimitMax    = 20
	rateLimitWindow = time.Second
)

// RateLimitMiddleware limits each client IP to 20 requests per second, so one
// noisy client cannot exhaust the quota of the others.
func RateLimitMiddleware() fiber.Handler {
	return rateLimit(rateLimitMax, rateLimitWindow, func(c *fiber.Ctx) string { return c.IP() })
}

func rateLimit(max int, window time.Duration, key func(*fiber.Ctx) string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   window,
		KeyGenerator: key,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Too Many Requests",
			})
		},
	})
}

// RequireAuth is a middleware that checks the X-API-Key header against the hashed APIKey table
func RequireAuth(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")

		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Missing X-API-Key header",
			})
		}

		var key models.APIKey
		hash256 := sha256.Sum256([]byte(apiKey))
		id := hex.EncodeToString(hash256[:])

		if err := db.Where("id = ?", id).First(&key).Error; err != nil || !models.VerifyAPIKey(apiKey, key.KeyHash) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: Invalid X-API-Key",
			})
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
