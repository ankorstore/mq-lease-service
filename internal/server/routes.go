package server

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server/handlers"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, orchestrator lease.ProviderOrchestrator) {
	app.Get("/", handlers.ProviderList(orchestrator)).Name("providers.list")

	providerRoutes := app.Group("/:owner/:repo/:baseRef").Name("provider.")
	providerRoutes.Post("/acquire", handlers.Acquire(orchestrator)).Name("acquire")
	providerRoutes.Post("/release", handlers.Release(orchestrator)).Name("release")
	providerRoutes.Get("/", handlers.ProviderDetails(orchestrator)).Name("show")
	providerRoutes.Delete("/", handlers.ProviderClear(orchestrator)).Name("clear")
}

func RegisterK8sProbesRoutes(app *fiber.App, storage storage.Storage[*lease.ProviderState]) {
	app.Get("/k8s/liveness", handlers.Liveness()).Name("k8s.liveness")
	app.Get("/k8s/readiness", handlers.Readiness(storage)).Name("k8s.readiness")
}
