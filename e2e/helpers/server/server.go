package server

import (
	"math/rand"

	"github.com/ankorstore/mq-lease-service/internal/server"
	"k8s.io/utils/clock"
)

// CreateAndInit creates a base API server (with a dummy logger) and with the provided dependencies
// the user will probably want to use pre-configured mocked services (for example the clock), or a custom storage path
func New(configPath string, persistentStateDir string, clock clock.PassiveClock) server.Server {
	return server.New(server.NewOpts{
		// the port isn't that important here, since we're not going to start it, but rather use fiber app.Test
		// methods to directly tests the httpHandlers
		Port:               rand.Intn(1000) + 10000, //nolint
		ConfigPath:         configPath,
		PersistentStateDir: persistentStateDir,
		Clock:              clock,
	})
}
