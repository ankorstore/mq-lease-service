package config

import (
	"os"
	"strconv"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
)

// Default values used in the env vars (which are exploited in the default configuration placeholders)
const (
	DefaultConfigRepoOwner                    = "e2e"
	DefaultConfigRepoName                     = "e2e-repo"
	DefaultConfigRepoBaseRef                  = "main"
	DefaultConfigRepoStabilizeDurationSeconds = 30
	DefaultConfigRepoExpectedRequestCount     = 4
	DefaultConfigRepoTTLSeconds               = 200
)

// baseConfigContent default YAML configuration used in GenerateDefaultConfig method
const baseConfigContent = `
repositories:
  - owner: ${E2E_CONFIG_REPO_OWNER}
    name: ${E2E_CONFIG_REPO_NAME}
    base_ref: ${E2E_CONFIG_REPO_BASE_REF}
    stabilize_duration_seconds: ${E2E_CONFIG_REPO_STABILIZE_DURATION_SECONDS}
    expected_request_count: ${E2E_CONFIG_REPO_EXPECTED_REQUEST_COUNT}
    ttl_seconds: ${E2E_CONFIG_REPO_TTL_SECONDS}
`

type HelperOption func() map[string]string

// WithRepoOwner override the owner value used in base configuration YAML (i.e. don't use the default one)
func WithRepoOwner(owner string) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_OWNER": owner,
		}
	}
}

// WithRepoName override the repo name value used in base configuration YAML (i.e. don't use the default one)
func WithRepoName(name string) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_NAME": name,
		}
	}
}

// WithBaseRef override the base ref value used in base configuration YAML (i.e. don't use the default one)
func WithBaseRef(baseRef string) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_BASE_REF": baseRef,
		}
	}
}

// WithStabilizeDurationSeconds override the Stabilize duration value used in base configuration YAML (i.e. don't use the default one)
func WithStabilizeDurationSeconds(duration int) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_STABILIZE_DURATION_SECONDS": strconv.Itoa(duration),
		}
	}
}

// WithExpectedRequestCount override the expected request value used in base configuration YAML (i.e. don't use the default one)
func WithExpectedRequestCount(count int) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_EXPECTED_REQUEST_COUNT": strconv.Itoa(count),
		}
	}
}

// WithTTLSeconds override the TTL value used in base configuration YAML (i.e. don't use the default one)
func WithTTLSeconds(duration int) HelperOption {
	return func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_TTL_SECONDS": strconv.Itoa(duration),
		}
	}
}

type Helper struct {
	baseDir      string
	setupEnvVars map[string]struct{}
}

// NewHelper will create a dedicated instance of the helper, and will create alongside of it a base temporary folder
// in the FS to host any further temporary configuration files
func NewHelper() *Helper {
	baseDir, err := os.MkdirTemp("", "e2e-gen-config-")
	if err != nil {
		panic(err)
	}
	return &Helper{
		baseDir:      baseDir,
		setupEnvVars: make(map[string]struct{}),
	}
}

// NewConfigFile is creating a new temporary file on the FS, with the config YAML content provided in the parameters.
// It returns the file name in return
func (h *Helper) NewConfigFile(yaml string) string {
	file, err := os.CreateTemp(h.baseDir, "*-config.yaml")
	if err != nil {
		panic(err)
	}
	filePath := file.Name()

	if _, err := file.WriteString(yaml); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}

	return filePath
}

// LoadConfig is loading the config object, based on the path given in arguments
func (h *Helper) LoadConfig(path string, options ...HelperOption) *latest.ServerConfig {
	for _, option := range options {
		for k, v := range option() {
			h.setupEnvVars[k] = struct{}{}
			_ = os.Setenv(k, v)
		}
	}

	cfg, err := config.LoadServerConfig(path)
	if err != nil {
		panic(err)
	}
	return cfg
}

// GenerateDefaultConfig is generating a default config file on the FS, using default YAML declared above
// in this package (baseConfigContent)
func (h *Helper) GenerateDefaultConfig() string {
	return h.NewConfigFile(baseConfigContent)
}

// LoadDefaultConfig is gluing calls to GenerateDefaultConfig and LoadConfig, while setting up the configuration
// placeholders values (which are using default constants as values) to use as env vars
// (which then will be used as part of the configuration parsing)
func (h *Helper) LoadDefaultConfig(options ...HelperOption) (*latest.ServerConfig, string) {
	baseOptions := func() map[string]string {
		return map[string]string{
			"E2E_CONFIG_REPO_OWNER":                      DefaultConfigRepoOwner,
			"E2E_CONFIG_REPO_NAME":                       DefaultConfigRepoName,
			"E2E_CONFIG_REPO_BASE_REF":                   DefaultConfigRepoBaseRef,
			"E2E_CONFIG_REPO_STABILIZE_DURATION_SECONDS": strconv.Itoa(DefaultConfigRepoStabilizeDurationSeconds),
			"E2E_CONFIG_REPO_EXPECTED_REQUEST_COUNT":     strconv.Itoa(DefaultConfigRepoExpectedRequestCount),
			"E2E_CONFIG_REPO_TTL_SECONDS":                strconv.Itoa(DefaultConfigRepoTTLSeconds),
		}
	}

	opts := []HelperOption{baseOptions}
	opts = append(opts, options...)
	configFile := h.GenerateDefaultConfig()

	return h.LoadConfig(configFile, opts...), configFile
}

// CleanupEnv will unset pre-declared env vars
func (h *Helper) CleanupEnv() {
	for k := range h.setupEnvVars {
		_ = os.Unsetenv(k)
	}
	h.setupEnvVars = map[string]struct{}{}
}

// Cleanup will delete the temporary config files created on the FS
func (h *Helper) Cleanup() {
	_ = os.RemoveAll(h.baseDir)
}
