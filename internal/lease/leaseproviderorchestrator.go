package lease

import (
	"errors"
	"fmt"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
)

func NewProviderOrchestrator(repositories []*latest.GithubRepositoryConfig) ProviderOrchestrator {
	leaseProviders := make(map[string]Provider)
	for _, repository := range repositories {
		leaseProviders[getKey(repository.Owner, repository.Name, repository.BaseRef)] = NewLeaseProvider(ProviderOpts{
			StabilizeDuration:    time.Second * time.Duration(repository.StabilizeDuration),
			TTL:                  time.Second * time.Duration(repository.TTL),
			ExpectedRequestCount: repository.ExpectedRequestCount,
		})
	}
	return &leaseProviderOrchestratorImpl{
		leaseProviders: leaseProviders,
	}
}

type ProviderOrchestrator interface {
	Get(owner string, repo string, baseRef string) (Provider, error)
	GetAll() map[string]Provider
}

type leaseProviderOrchestratorImpl struct {
	leaseProviders map[string]Provider
}

func (o *leaseProviderOrchestratorImpl) GetAll() map[string]Provider {
	return o.leaseProviders
}

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
