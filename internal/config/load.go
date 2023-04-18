package config

import (
	"os"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
	"github.com/drone/envsubst/v2"
	"gopkg.in/yaml.v3"
)

// LoadServerConfig opens the configuration file, performs environment substitution and parses it.
// The environment substitution allows to e.g. include private information in form of
// ${MY_GITHUB_PRIVATE_KEY} rather than hardcoding it on the configuration
func LoadServerConfig(path string) (*latest.ServerConfig, error) {
	serverConfig := &latest.ServerConfig{}
	err := load(path, serverConfig)
	return serverConfig, err
}

func load(path string, config interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	templated, err := envsubst.EvalEnv(string(raw))
	if err != nil {
		return err
	}

	return yaml.Unmarshal([]byte(templated), config)
}
