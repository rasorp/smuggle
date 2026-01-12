BUILD_COMMIT := $(shell git rev-parse HEAD)
BUILD_DIRTY := $(if $(shell git status --porcelain),+CHANGES)
BUILD_COMMIT_FLAG := github.com/rasorp/smuggle/internal/version.BuildCommit=$(BUILD_COMMIT)$(BUILD_DIRTY)

BUILD_TIME ?= $(shell TZ=UTC0 git show -s --format=%cd --date=format-local:'%Y-%m-%dT%H:%M:%SZ' HEAD)
BUILD_TIME_FLAG := github.com/rasorp/smuggle/internal/version.BuildTime=$(BUILD_TIME)

# Populate the ldflags using the Git commit information and and build time
# which will be present in the binary version output.
GO_LDFLAGS = -X $(BUILD_COMMIT_FLAG) -X $(BUILD_TIME_FLAG)

# Disable CGO which is not required for Smuggle.
CGO_ENABLED = 0

bin/%/smuggle: GO_OUT ?= $@
bin/%/smuggle: ## Build Smuggle for GOOS & GOARCH; eg. bin/linux_amd64/smuggle
	@echo "==> Building $@..."
	@GOOS=$(firstword $(subst _, ,$*)) \
		GOARCH=$(lastword $(subst _, ,$*)) \
		go build \
		-o $(GO_OUT) \
		-trimpath \
		-ldflags "$(GO_LDFLAGS)" \
		cmd/cmd.go
	@echo "==> Done"

.PHONY: build
build: ## Build a development version of Smuggle
	@echo "==> Building Smuggle..."
	@go build \
		-o ./bin/smuggle \
		-trimpath \
		-ldflags "$(GO_LDFLAGS)" \
		cmd/cmd.go
	@echo "==> Done"

.PHONY: docker-build
docker-build: ## Build a Docker image for Smuggle
	@echo "==> Building Smuggle Docker image..."
	@docker build -f build/Docker/Dockerfile .
	@echo "==> Done"

.PHONY: lint
lint: ## Run linters against the Smuggle codebase
	@echo "==> Linting Smuggle..."
	@golangci-lint run --config=build/lint/golangci.yaml ./...
	@echo "==> Done"

.PHONY: test
test: ## Run tests against the Smuggle codebase
	@echo "==> Testing Smuggle..."
	@gotestsum --format=testname -- -race ./...
	@echo "==> Done"

HELP_FORMAT="    \033[36m%-22s\033[0m %s\n"
.PHONY: help
help: ## Display this usage information
	@echo "Valid targets:"
	@grep -E '^[^ ]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; \
			{printf $(HELP_FORMAT), $$1, $$2}'
	@echo ""
