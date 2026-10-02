package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"machdown/server/services"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

type APIController struct {
	downloadService *services.DownloadService
}

func NewAPIController(ds *services.DownloadService) *APIController {
	return &APIController{downloadService: ds}
}

type EnqueueRequest struct {
	URL           string   `json:"url"`
	TargetClients []string `json:"target_clients"`
	Cookies       string   `json:"cookies"`
	UserAgent     string   `json:"user_agent"`
}

func (ctrl *APIController) HandleEnqueue(c *fiber.Ctx) error {
	var req EnqueueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON body"})
	}

	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "URL is required"})
	}

	targetClientsJSON, _ := json.Marshal(req.TargetClients)

	job, err := ctrl.downloadService.Enqueue(req.URL, string(targetClientsJSON), req.Cookies, req.UserAgent)
	if err != nil {
		if err.Error() == "URL already exists or is downloading" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(job)
}

func (ctrl *APIController) HandleListCompleted(c *fiber.Ctx) error {
	clientID := c.Query("client_id")
	if clientID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "client_id required"})
	}
	jobs, err := ctrl.downloadService.GetCompletedJobs(clientID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(jobs)
}

// HandleDownloadFile serve o arquivo de um job pelo seu ID.
// BUG FIX: antes usava path hardcoded "downloads/"+jobID que não respeitava
// o storagePath configurado. Agora usa o DiskManager via GetFilePath.
func (ctrl *APIController) HandleDownloadFile(c *fiber.Ctx) error {
	jobID := c.Params("id")
	path := ctrl.downloadService.GetFilePath(jobID)
	if path == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "File not found"})
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "File not found on disk"})
	}
	return c.SendFile(path)
}

func (ctrl *APIController) HandleMarkSynced(c *fiber.Ctx) error {
	jobID := c.Params("id")
	err := ctrl.downloadService.MarkJobSynced(jobID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (ctrl *APIController) HandlePauseDownload(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if err := ctrl.downloadService.PauseDownload(jobID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (ctrl *APIController) HandleResumeDownload(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if err := ctrl.downloadService.ResumeDownload(jobID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

// HandleSSEProgress é o endpoint Server-Sent Events para progresso de downloads em tempo real.
// Os clientes conectam a GET /api/downloads/progress e recebem eventos JSON enquanto
// os chunks são baixados. Isso elimina a necessidade de polling no client.
func (ctrl *APIController) HandleSSEProgress(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // desabilita buffering em Nginx reverse proxies

	ch := ctrl.downloadService.Hub.Subscribe()

	// Fiber/fasthttp usa SetBodyStreamWriter para streaming verdadeiro sem buffer
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer ctrl.downloadService.Hub.Unsubscribe(ch)
		for evt := range ch {
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			// Formato SSE: "data: <json>\n\n"
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return // cliente desconectou
			}
			if err := w.Flush(); err != nil {
				return // cliente desconectou
			}
		}
	}))

	return nil
}

func (ctrl *APIController) HandleEvents(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ch := ctrl.downloadService.Hub.Subscribe()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer ctrl.downloadService.Hub.Unsubscribe(ch)
		for evt := range ch {
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: job-update\n")
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))

	return nil
}
