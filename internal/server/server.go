package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/metrics"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/server/middlewares"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/version"
	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/utils/clock"
)

type Server interface {
	// Init set up the API server dependencies, open a connection to the storage, register the API routes, etc.
	Init() error
	// Start is starting the server listening loop
	Start() error
	// Shutdown will attempt to do a graceful shutdown of the server, and will clean up the dependencies (close the storage, etc...)
	Shutdown() error

	// Testing

	// Test should be called to test an API endpoint. This will relay the call to fiber app.Test() method. (TESTING)
	Test(req *http.Request, msTimeout ...int) (*http.Response, error)
	// GetOrchestrator is returning the current instance of the lease providers orchestrator (TESTING)
	GetOrchestrator() lease.ProviderOrchestrator
}

type NewOpts struct {
	Port               int
	Logger             *zerolog.Logger
	ConfigPath         string
	PersistentStateDir string
	Clock              clock.PassiveClock
}

// New returns a server instance
func New(opts NewOpts) Server {
	return &serverImpl{
		port:               opts.Port,
		configPath:         opts.ConfigPath,
		logger:             opts.Logger,
		persistentStateDir: opts.PersistentStateDir,
		clock:              opts.Clock,
	}
}

type serverImpl struct {
	port               int
	configPath         string
	persistentStateDir string
	storage            storage.Storage[*lease.ProviderState]
	app                *fiber.App
	logger             *zerolog.Logger
	ctx                context.Context
	clock              clock.PassiveClock
	orchestrator       lease.ProviderOrchestrator
	initialized        bool
}

// Init set up the API server dependencies, open a connection to the storage, register the API routes, etc.
func (s *serverImpl) Init() error {
	if s.initialized {
		return errors.New("server already initialized")
	}
	s.initialized = true

	s.ctx = s.logger.WithContext(context.Background())

	// Setup state storage
	s.storage = storage.New[*lease.ProviderState](s.ctx, s.persistentStateDir)
	if err := s.storage.Init(); err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
	}

	//  defer the closing of the storage if anything is panicking in the rest of the Init method
	defer func() {
		if r := recover(); r != nil {
			log.Ctx(s.ctx).Warn().Msg("Panicking at server initialisation. Try to gracefully close the storage.")
			if err := s.storage.Close(); err != nil {
				log.Ctx(s.ctx).Error().Err(err).Msg("Failed to close storage")
			}
			panic(r)
		}
	}()

	// Load config
	cfg, err := config.LoadServerConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("failed loading configuration: %w", err)
	}

	// Metrics
	promRegistry := prometheus.NewRegistry()
	metricsServ := metrics.New(metrics.NewOpts{
		AppName:        version.Version{}.GetAppName(),
		PromRegisterer: promRegistry,
		PromGatherer:   promRegistry,
	})
	metricsServ.AddDefaultCollectors()

	// Lease provider orchestrator (handling all repos merge queue leases)
	s.orchestrator = lease.NewProviderOrchestrator(lease.NewProviderOrchestratorOpts{
		Repositories: cfg.Repositories,
		Clock:        s.clock,
		Storage:      s.storage,
		Metrics:      metricsServ,
	})
	// tries to hydrate the states of managed providers from the storage
	if err := s.orchestrator.HydrateFromState(s.ctx); err != nil {
		return fmt.Errorf("failed to hydrate orchestrator providers from state: %w", err)
	}

	// Fiber app configuration
	s.app = fiber.New(fiber.Config{DisableStartupMessage: true})
	s.app.Use(middlewares.PrometheusMiddleware(
		s.app,
		metricsServ,
		"/metrics",
	))
	s.app.Use(middlewares.LoggerMiddleware(s.logger))
	// recover middleware allow us to avoid a panic (happening in middlewares or http handlers) to stop the server
	// this will result in a 500, but the server will continue to accept requests.
	s.app.Use(fiberrecover.New(fiberrecover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			log.Ctx(c.UserContext()).Error().Msgf("panic: %v\n%s", e, debug.Stack())
		},
	}))

	// register API routes on the fiber app
	RegisterRoutes(s.app, s.orchestrator)

	return nil
}

// Start is starting the server listening loop
func (s *serverImpl) Start() error {
	if !s.initialized {
		return errors.New("server has not been initialized")
	}

	log.Ctx(s.ctx).Info().Msg("Starting server...")
	return s.app.Listen(":" + strconv.Itoa(s.port))
}

// Shutdown will attempt to do a graceful shutdown of the server, and will clean up the dependencies (close the storage, etc...)
func (s *serverImpl) Shutdown() error {
	if !s.initialized {
		return errors.New("server has not been initialized")
	}

	log.Ctx(s.ctx).Warn().Msg("Gracefully shutting down...")
	defer func(s *serverImpl) {
		// tries to close the storage
		if err := s.storage.Close(); err != nil {
			log.Ctx(s.ctx).Error().Err(err).Msg("Failed to close storage")
		} else {
			log.Ctx(s.ctx).Warn().Msg("Storage closed gracefully")
		}
	}(s)
	// inform Fiber about our intention to shut down. This allows it to gracefully shutdown
	// (with a grace period of 3 seconds for currently executing requests)
	return s.app.ShutdownWithTimeout(3 * time.Second)
}

// Test should be called to test an API endpoint. This will relay the call to fiber app.Test() method. (TESTING)
func (s *serverImpl) Test(req *http.Request, msTimeout ...int) (*http.Response, error) {
	return s.app.Test(req, msTimeout...)
}

// GetOrchestrator is returning the current instance of the lease providers orchestrator (TESTING)
func (s *serverImpl) GetOrchestrator() lease.ProviderOrchestrator {
	return s.orchestrator
}
