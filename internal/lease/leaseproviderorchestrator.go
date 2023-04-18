package lease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/metrics"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/utils/clock"
)

type NewProviderOrchestratorOpts struct {
	Repositories []*latest.GithubRepositoryConfig
	Clock        clock.PassiveClock
	Storage      storage.Storage[*ProviderState]
	Metrics      metrics.Metrics
}

type providerMetrics struct {
	queueSize       *prometheus.GaugeVec
	mergedBatchSize *prometheus.HistogramVec
}

func NewProviderOrchestrator(opts NewProviderOrchestratorOpts) ProviderOrchestrator {
	var pMetrics *providerMetrics
	if opts.Metrics != nil {
		pMetrics = &providerMetrics{
			queueSize: opts.Metrics.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "provider_lease_requests_total",
					Help: "All lease requests known in a provider",
				},
				[]string{"provider_id"},
			),
			mergedBatchSize: opts.Metrics.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "provider_merged_batch_size",
					Help:    "Number of requests merged in same batch",
					Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 10, 15, 20},
				},
				[]string{"provider_id"},
			),
		}
	}

	leaseProviders := make(map[string]Provider)
	for _, repository := range opts.Repositories {
		key := getKey(repository.Owner, repository.Name, repository.BaseRef)
		leaseProviders[key] = NewLeaseProvider(ProviderOpts{
			StabilizeDuration:    time.Second * time.Duration(repository.StabilizeDuration),
			TTL:                  time.Second * time.Duration(repository.TTL),
			ExpectedRequestCount: repository.ExpectedRequestCount,
			DelayAssignmentCount: repository.DelayLeaseAssignmentBy,
			ID:                   key,
			Clock:                opts.Clock,
			Storage:              opts.Storage,
			Metrics:              pMetrics,
		})
	}
	return &leaseProviderOrchestratorImpl{
		leaseProviders: leaseProviders,
	}
}

// ProviderOrchestrator the orchestrator is a registry of lease Providers.
// it allows the system to be able to handle multiple repositories (and or multiple merge queues per repos, which
// are not targeting the same base ref)
type ProviderOrchestrator interface {
	// Get returns a specific lease provider
	Get(owner string, repo string, baseRef string) (Provider, error)
	// GetAll returns all managed lease providers
	GetAll() map[string]Provider
	// HydrateFromState will recursively hydrate all the states of managed providers
	HydrateFromState(ctx context.Context) error
}

type leaseProviderOrchestratorImpl struct {
	leaseProviders map[string]Provider
}

// HydrateFromState will recursively hydrate all the states of managed providers
func (o *leaseProviderOrchestratorImpl) HydrateFromState(ctx context.Context) error {
	for _, provider := range o.leaseProviders {
		if err := provider.HydrateFromState(ctx); err != nil {
			return err
		}
	}
	return nil
}

// GetAll returns all managed lease providers
func (o *leaseProviderOrchestratorImpl) GetAll() map[string]Provider {
	return o.leaseProviders
}

// Get returns a specific lease provider
func (o *leaseProviderOrchestratorImpl) Get(owner string, repo string, baseRef string) (Provider, error) {
	key := getKey(owner, repo, baseRef)
	if provider, ok := o.leaseProviders[key]; ok {
		return provider, nil
	}

	return nil, errors.New("unknown provider")
}

func getKey(owner string, repo string, baseRef string) string {
	return fmt.Sprintf("%s:%s:%s", owner, repo, baseRef)
}
