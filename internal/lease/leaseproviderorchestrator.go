package lease

import (
	"fmt"
	"time"
)

const (
	StabilizeDuration    = time.Minute * 5
	TTL                  = time.Second * 30
	ExpectedRequestCount = 4
)

func NewLeaseProviderOrchestrator() LeaseProviderOrchestrator {
	return &leaseProviderOrchestratorImpl{
		leaseProviders: make(map[string]LeaseProvider),
	}
}

type LeaseProviderOrchestrator interface {
	Get(owner string, repo string, baseRef string) LeaseProvider
}

type leaseProviderOrchestratorImpl struct {
	leaseProviders map[string]LeaseProvider
}

func (o *leaseProviderOrchestratorImpl) Get(owner string, repo string, baseRef string) LeaseProvider {
	key := getKey(owner, repo, baseRef)
	if provider, ok := o.leaseProviders[key]; ok {
		return provider
	}

	o.leaseProviders[key] = NewLeaseProvider(LeaseProviderOpts{
		StabilizeDuration:    StabilizeDuration,
		TTL:                  TTL,
		ExpectedRequestCount: ExpectedRequestCount,
	})

	return o.leaseProviders[key]
}

func getKey(owner string, repo string, baseRef string) string {
	return fmt.Sprintf("%s:%s:%s", owner, repo, baseRef)
}
