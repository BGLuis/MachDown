package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"machdown/server/models"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func newAuthApp(t *testing.T, validKey string) *fiber.App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.AutoMigrate(&models.APIKey{}, &models.Client{})

	id, hash, prefix := models.GenerateAPIKeyData(validKey)
	db.Create(&models.APIKey{ID: id, KeyHash: hash, KeyPrefix: prefix, Name: "test"})

	app := fiber.New()
	app.Use(RequireAuth(db))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	return app
}

func statusFor(t *testing.T, app *fiber.App, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestRequireAuth(t *testing.T) {
	app := newAuthApp(t, "correct-key")

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"valid key", map[string]string{"X-API-Key": "correct-key"}, http.StatusOK},
		{"wrong key", map[string]string{"X-API-Key": "wrong-key"}, http.StatusUnauthorized},
		{"missing key", nil, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		if got := statusFor(t, app, tc.headers); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestRateLimitIsPerClient(t *testing.T) {
	app := fiber.New()
	app.Use(rateLimit(2, time.Minute, func(c *fiber.Ctx) string { return c.Get("X-Test-Client") }))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	noisy := map[string]string{"X-Test-Client": "noisy"}
	quiet := map[string]string{"X-Test-Client": "quiet"}

	statusFor(t, app, noisy)
	statusFor(t, app, noisy)
	if got := statusFor(t, app, noisy); got != http.StatusTooManyRequests {
		t.Fatalf("third request from the noisy client: got %d, want 429", got)
	}
	if got := statusFor(t, app, quiet); got != http.StatusOK {
		t.Fatalf("a different client must keep its own quota: got %d, want 200", got)
	}
}
