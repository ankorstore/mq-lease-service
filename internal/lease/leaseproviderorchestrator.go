package lease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/storage"
	"k8s.io/utils/clock"
)

type NewProviderOrchestratorOpts struct {
	Repositories []*latest.GithubRepositoryConfig
	Clock        clock.PassiveClock
	Storage      storage.Storage[*ProviderState]
}

func NewProviderOrchestrator(opts NewProviderOrchestratorOpts) ProviderOrchestrator {
	leaseProviders := make(map[string]Provider)
	for _, repository := range opts.Repositories {
		key := getKey(repository.Owner, repository.Name, repository.BaseRef)
		leaseProviders[key] = NewLeaseProvider(ProviderOpts{
			StabilizeDuration:    time.Second * time.Duration(repository.StabilizeDuration),
			TTL:                  time.Second * time.Duration(repository.TTL),
			ExpectedRequestCount: repository.ExpectedRequestCount,
			ID:                   key,
			Clock:                opts.Clock,
			Storage:              opts.Storage,
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
