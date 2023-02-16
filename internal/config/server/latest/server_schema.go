package latest

// ServerConfig represents the current server configuration file.
type ServerConfig struct {
	Repositories []*GithubRepositoryConfig `yaml:"repositories,omitempty"`
}

// GithubRepositoryConfig defines how a repository should be handled
type GithubRepositoryConfig struct {
	Owner                string `yaml:"owner"`
	Name                 string `yaml:"name"`
	BaseRef              string `yaml:"base_ref"`
	StabilizeDuration    int    `yaml:"stabilize_duration"`
	ExpectedRequestCount int    `yaml:"expected_request_count"`
}
