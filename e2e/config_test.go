package e2e_test

import (
	"github.com/ankorstore/mq-lease-service/e2e/helpers/config"
	"github.com/ankorstore/mq-lease-service/internal/config/server/latest"
	. "github.com/onsi/ginkgo/v2" //nolint
	. "github.com/onsi/gomega" //nolint
)

var _ = Describe("Config", Ordered, func() {
	var configHelper *config.Helper
	BeforeAll(func() {
		configHelper = config.NewHelper()
	})

	Describe("CanLoadConfig", func() {
		Context("with default config", func() {
			It("should load the config", func() {
				var cfg *latest.ServerConfig
				cfg, _ = configHelper.LoadDefaultConfig()
				Expect(cfg).To(Equal(&latest.ServerConfig{Repositories: []*latest.GithubRepositoryConfig{
					{
						Owner:                  config.DefaultConfigRepoOwner,
						Name:                   config.DefaultConfigRepoName,
						BaseRef:                config.DefaultConfigRepoBaseRef,
						StabilizeDuration:      config.DefaultConfigRepoStabilizeDurationSeconds,
						ExpectedRequestCount:   config.DefaultConfigRepoExpectedRequestCount,
						TTL:                    config.DefaultConfigRepoTTLSeconds,
						DelayLeaseAssignmentBy: config.DefaultConfigRepoDelayAssignmentCount,
					},
				}}))

				cfg, _ = configHelper.LoadDefaultConfig(
					config.WithRepoName("another-repo"),
					config.WithBaseRef("develop"),
					config.WithStabilizeDurationSeconds(3),
				)
				Expect(cfg).To(Equal(&latest.ServerConfig{Repositories: []*latest.GithubRepositoryConfig{
					{
						Owner:                  config.DefaultConfigRepoOwner,
						Name:                   "another-repo",
						BaseRef:                "develop",
						StabilizeDuration:      3,
						ExpectedRequestCount:   config.DefaultConfigRepoExpectedRequestCount,
						TTL:                    config.DefaultConfigRepoTTLSeconds,
						DelayLeaseAssignmentBy: config.DefaultConfigRepoDelayAssignmentCount,
					},
				}}))
			})
		})
	})

	AfterAll(func() {
		configHelper.Cleanup()
	})
})
