package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/ankorstore/mq-lease-service/internal/config"
	"github.com/ankorstore/mq-lease-service/internal/lease"
	"github.com/ankorstore/mq-lease-service/internal/metrics"
	"github.com/ankorstore/mq-lease-service/internal/server/middlewares"
	"github.com/ankorstore/mq-lease-service/internal/storage"
	"github.com/ankorstore/mq-lease-service/internal/version"
	"github.com/gofiber/fiber/v2"
	fiberbasicauth "github.com/gofiber/fiber/v2/middleware/basicauth"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"k8s.io/utils/clock"
)

type Server interface {
	// Run the server
	Run(ctx context.Context) error

	// Testing
	RunTest(ctx context.Context) error
	WaitReady(ctx context.Context) bool

	// Test should be called to test an API endpoint. This will relay the call to fiber app.Test() method. (TESTING)
	Test(req *http.Request, msTimeout ...int) (*http.Response, error)
	// GetOrchestrator is returning the current instance of the lease providers orchestrator (TESTING)
	GetOrchestrator() lease.ProviderOrchestrator
}

type NewOpts struct {
	Port               int
	ConfigPath         string
	PersistentStateDir string
	Clock              clock.PassiveClock
}

// New returns a server instance
func New(opts NewOpts) Server {
	return &serverImpl{
		waitReady:          make(chan struct{}, 1),
		port:               opts.Port,
		configPath:         opts.ConfigPath,
		persistentStateDir: opts.PersistentStateDir,
		clock:              opts.Clock,
	}
}

type serverImpl struct {
	waitReady          chan struct{}
	port               int
	configPath         string
	persistentStateDir string
	storage            storage.Storage[*lease.ProviderState]
	app                *fiber.App
	clock              clock.PassiveClock
	orchestrator       lease.ProviderOrchestrator
}

func (s *serverImpl) WaitReady(ctx context.Context) bool {
	select {
	case <-s.waitReady:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *serverImpl) setup(ctx context.Context) error {
	// Make sure we mark the server as ready before returning (this does not cover errors, in the setup process, they need to be checked separately)
	defer close(s.waitReady)

	// Setup state storage
	s.storage = storage.New[*lease.ProviderState](ctx, s.persistentStateDir)
	if err := s.storage.Init(); err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
	}

	//  defer the closing of the storage if anything is panicking in the rest of the Init method
	defer func() {
		if r := recover(); r != nil {
			log.Ctx(ctx).Warn().Msg("Panicking at server initialisation. Try to gracefully close the storage.")
			if err := s.storage.Close(); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("Failed to close storage")
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
	if err := s.orchestrator.HydrateFromState(ctx); err != nil {
		return fmt.Errorf("failed to hydrate orchestrator providers from state: %w", err)
	}

	// Fiber app configuration
	s.app = fiber.New(fiber.Config{DisableStartupMessage: true})
	s.app.Use(middlewares.PrometheusMiddleware(
		s.app,
		metricsServ,
		"/metrics",
	))
	s.app.Use(middlewares.LoggerMiddleware(log.Ctx(ctx)))
	// recover middleware allow us to avoid a panic (happening in middlewares or http handlers) to stop the server
	// this will result in a 500, but the server will continue to accept requests.
	s.app.Use(fiberrecover.New(fiberrecover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			log.Ctx(c.UserContext()).Error().Msgf("panic: %v\n%s", e, debug.Stack())
		},
	}))

	// Configure basic auth if needed
	if cfg.AuthConfig != nil && cfg.AuthConfig.BasicAuth != nil {
		log.Ctx(ctx).Info().Msg("Basic auth enabled")
		s.app.Use(fiberbasicauth.New(fiberbasicauth.Config{
			Users: cfg.AuthConfig.BasicAuth.Users,
		}))
	}

	// register k8s probes handlers
	RegisterK8sProbesRoutes(s.app, s.storage)
	// register API routes on the fiber app
	RegisterRoutes(s.app, s.orchestrator)

	return nil
}

// RunTest runs the server in test mode (actually does not listen)
func (s *serverImpl) RunTest(ctx context.Context) error {
	err := s.setup(ctx)
	if err != nil {
		return err
	}
	<-ctx.Done()
	return s.storage.Close()
}

// Run operates the lease server
func (s *serverImpl) Run(ctx context.Context) error {
	err := s.setup(ctx)
	if err != nil {
		return err
	}

	// Run Server and shtudown on context cancel
	grp, runCtx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		log.Ctx(ctx).Info().Int("port", s.port).Msg("Starting server")
		return s.app.Listen(":" + strconv.Itoa(s.port))
	})
	grp.Go(func() error {
		<-runCtx.Done()

		log.Ctx(ctx).Warn().Msg("Shutting down fiber app")
		shutDownErr := s.app.ShutdownWithTimeout(10 * time.Second)

		log.Ctx(ctx).Warn().Msg("Closign storage")
		storageErr := s.storage.Close()

		return errors.Join(shutDownErr, storageErr)
	})

	return grp.Wait()
}

// Test should be called to test an API endpoint. This will relay the call to fiber app.Test() method. (TESTING)
func (s *serverImpl) Test(req *http.Request, msTimeout ...int) (*http.Response, error) {
	return s.app.Test(req, msTimeout...)
}

// GetOrchestrator is returning the current instance of the lease providers orchestrator (TESTING)
func (s *serverImpl) GetOrchestrator() lease.ProviderOrchestrator {
	return s.orchestrator
}
