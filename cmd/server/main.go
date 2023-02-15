package main

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/version"
	"github.com/ankorstore/gh-action-mq-lease-service/pkg/util/logger"
	"github.com/gofiber/fiber/v2"
)

const (
	LISTEN = ":8080"
)

func main() {

	log := logger.New(version.Version{})
	app := fiber.New()
	app.Use(logger.FiberMiddleware(log))

	if err := app.Listen(LISTEN); err != nil {
		log.Err(err).Msg("Fiber server failed")
	}

}
