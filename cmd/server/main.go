package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/ankorstore/mq-lease-service/internal/server"
	"github.com/ankorstore/mq-lease-service/internal/version"
	"github.com/ankorstore/mq-lease-service/pkg/util/logger"
)

var (
	serverPort         uint
	configPath         string
	logDebug           bool
	logJSON            bool
	persistentStateDir string
)

func init() {
	// General flags
	flag.UintVar(&serverPort, "port", 9000, "server listening port")
	flag.StringVar(&configPath, "config", "./config.yaml", "Configuration path")

	// Logging flags
	flag.BoolVar(&logDebug, "log-debug", false, "Enable debug logging")
	flag.BoolVar(&logJSON, "log-json", true, "Enable console logging format")

	// Persistent state flags
	flag.StringVar(&persistentStateDir, "persistents-state-dir", "/tmp/state", "Setup the directory for persistent state storage")
}

func main() {
	flag.Parse()

	// Logger
	log := logger.New(logger.NewOpts{
		AppInfo: version.Version{},
		Debug:   logDebug,
		JSON:    logJSON,
	})

	// Main server
	s := server.New(server.NewOpts{
		Port:               int(serverPort),
		Logger:             &log,
		ConfigPath:         configPath,
		PersistentStateDir: persistentStateDir,
	})
	if err := s.Init(); err != nil {
		log.Panic().Err(err).Msg("Failed initializing the server")
	}

	// Signal handling (SIGTERM) to be able to gracefully shut down the server (both fiber + other resources,
	// like the storage for ex).
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	globShutdown := make(chan struct{}, 1)

	go func() {
		<-c
		log.Warn().Msg("SIGTERM received")
		if err := s.Shutdown(); err != nil {
			log.Error().Err(err).Msg("Could not shutdown server gracefully")
		}
		globShutdown <- struct{}{}
	}()

	if err := s.Start(); err != nil {
		log.Panic().Err(err).Msg("Server error")
	}

	<-globShutdown
}
