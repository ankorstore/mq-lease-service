DEBUG ?= false
LEASE_SERVICE_PORT ?= 9000

GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

E2E_GINKGO_VERSION := v2.8.2

default: clean lint test build

.PHONY: help
help: ## Print this help with list of available commands/targets and their purpose
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[36m\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

lint:  ## run linters on the code base
	golangci-lint run

clear-state:  ## cleanup storage DB
	rm -rf /tmp/state

.PHONY: clean
clean:  ## cleanup test cover output
	rm -rf cover.out

test:  ## run unit tests
	go test -race -cover $(shell go list ./... | grep -v /e2e)

.PHONY: e2e
e2e:  ## run e2e tests
	cd e2e && \
		go run github.com/onsi/ginkgo/v2/ginkgo@$(E2E_GINKGO_VERSION)

.PHONY: build
build:  ## build the application (server & cli)
	goreleaser build --snapshot --clean --single-target

run-server: build  ## run the server with an example config
	 ./dist/server_${GOOS}_${GOARCH}/server -config=hack/example-server-config.yaml -log-json=false -log-debug=$(DEBUG) -port=$(LEASE_SERVICE_PORT)
