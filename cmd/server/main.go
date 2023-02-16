package main

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server/handlers"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/version"
	"github.com/ankorstore/gh-action-mq-lease-service/pkg/util/logger"
	"github.com/gofiber/fiber/v2"
)

const (
	LISTEN = ":9999"
)

func main() {

	log := logger.New(version.Version{})
	app := fiber.New()
	app.Use(logger.FiberMiddleware(log))

	orchestrator := lease.NewLeaseProviderOrchestrator()

	app.Post("/:owner/:repo/:baseRef/acquire", handlers.Acquire(orchestrator))
	app.Post("/:owner/:repo/:baseRef/release", handlers.Release(orchestrator))

	app.Get("/:owner/:repo/:baseRef", handlers.ProviderDetails(orchestrator))

	if err := app.Listen(LISTEN); err != nil {
		log.Err(err).Msg("Fiber server failed")
	}
}
