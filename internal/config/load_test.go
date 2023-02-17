package config_test

import (
	"os"
	"testing"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/config"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/config/server/latest"
	"github.com/google/go-cmp/cmp"
)

const (
	TestServerYaml = `repositories:
  - owner: test
    name: repo0
    base_ref: main
    stabilize_duration: 300
    expected_request_count: 4
    ttl: 20
  - owner: test
    name: repo1
    base_ref: develop
    stabilize_duration: 100
    expected_request_count: 5
    ttl: 30`
)

func TestLoadServerConfig(t *testing.T) {
	var err error

	expected := latest.ServerConfig{
		Repositories: []*latest.GithubRepositoryConfig{
			{
				Owner:                "test",
				Name:                 "repo0",
				BaseRef:              "main",
				StabilizeDuration:    300,
				ExpectedRequestCount: 4,
				TTL:                  20,
			},
			{
				Owner:                "test",
				Name:                 "repo1",
				BaseRef:              "develop",
				StabilizeDuration:    100,
				ExpectedRequestCount: 3,
				TTL:                  30,
			},
		},
	}

	yamlFileName := prepareYamlFile(TestServerYaml)

	// Load config
	got, err := config.LoadServerConfig(yamlFileName)
	if err != nil {
		t.Errorf("Could not load config, %v", err)
	}
	if cmp.Equal(got, expected) {
		t.Errorf(cmp.Diff(got, err))
	}

	cleanup(yamlFileName)
}

func prepareYamlFile(content string) string {
	// Set up our test file
	f, err := os.CreateTemp("/tmp", "gotest")
	if err != nil {
		panic(err)
	}
	yamlFileName := f.Name()
	_, err = f.WriteString(content)
	if err != nil {
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}

	return yamlFileName
}

func cleanup(path string) {
	_ = os.Remove(path)
}
