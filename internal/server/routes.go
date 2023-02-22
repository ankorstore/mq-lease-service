package server

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server/handlers"
	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, orchestrator lease.ProviderOrchestrator) {
	app.Get("/", handlers.ProviderList(orchestrator)).Name("providers.list")

	providerRoutes := app.Group("/:owner/:repo/:baseRef").Name("provider.")
	providerRoutes.Post("/acquire", handlers.Acquire(orchestrator)).Name("acquire")
	providerRoutes.Post("/release", handlers.Release(orchestrator)).Name("release")
	providerRoutes.Get("/", handlers.ProviderDetails(orchestrator)).Name("show")
}
