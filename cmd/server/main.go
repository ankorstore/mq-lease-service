package main

import (
	"flag"
	"os"
	"strconv"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/version"
	"github.com/ankorstore/gh-action-mq-lease-service/pkg/util/logger"
	"github.com/gofiber/fiber/v2"
)

var (
	serverPort uint
	configPath string
)

func init() {
	flag.UintVar(&serverPort, "port", 9000, "server listening port")
	flag.StringVar(&configPath, "config", "./config.yaml", "Configuration path")

	// Register logging flags
	logger.InitFlags()
}

func main() {
	flag.Parse()

	// Logger
	log := logger.New(version.Version{})

	// Config
	cfg, err := config.LoadServerConfig(configPath)
	if err != nil {
		log.Error().Msg("Failed loading configuration")
		os.Exit(1)
	}

	// Lease provider orchestrator (handling all repos merge queue leases)
	orchestrator := lease.NewProviderOrchestrator(cfg.Repositories)

	// Fiber app
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(logger.FiberMiddleware(log))
	server.RegisterRoutes(app, orchestrator)

	log.Info().Msg("Bootstrap completed, starting...")

	if err := app.Listen(":" + strconv.Itoa(int(serverPort))); err != nil {
		log.Err(err).Msg("Fiber server failed")
		defer os.Exit(1)
	}
}
