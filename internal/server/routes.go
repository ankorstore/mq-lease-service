package server

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server/handlers"
	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, orchestrator lease.ProviderOrchestrator) {
	app.Get("/", handlers.ProviderList(orchestrator))

	providerRoutes := app.Group("/:owner/:repo/:baseRef")
	providerRoutes.Post("/acquire", handlers.Acquire(orchestrator))
	providerRoutes.Post("/release", handlers.Release(orchestrator))
	providerRoutes.Get("/", handlers.ProviderDetails(orchestrator))
}
