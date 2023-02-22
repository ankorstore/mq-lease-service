package server

import (
	"github.com/ankorstore/gh-action-mq-lease-service/e2e/helpers/logger"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server"
	"k8s.io/utils/clock"
)

// CreateAndInit creates a base API server (with a dummy logger) and with the provided dependencies
// the user will probably want to use pre-configured mocked services (for example the clock), or a custom storage path
func CreateAndInit(configPath string, persistentStateDir string, clock clock.PassiveClock) server.Server {
	s := server.New(server.NewOpts{
		// the port isn't that important here, since we're not going to start it, but rather use fiber app.Test
		// methods to directly tests the httpHandlers
		Port:               9999,
		Logger:             logger.NewDummyLogger(),
		ConfigPath:         configPath,
		PersistentStateDir: persistentStateDir,
		Clock:              clock,
	})
	if err := s.Init(); err != nil {
		panic(err)
	}

	return s
}

// Cleanup will attempt to do a graceful shutdown of the server
func Cleanup(srv server.Server) {
	if err := srv.Shutdown(); err != nil {
		panic(err)
	}
}
