# wuxing — developer task runner.
#
# On Windows, run these targets from Git Bash or WSL, or invoke the underlying
# `go`/`golangci-lint` commands directly from PowerShell.

GO            ?= go
GOLANGCI_LINT ?= golangci-lint
BIN_DIR       := bin
LDFLAGS       := -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all build test test-integration lint fmt run clean docker tidy

all: lint test build

## build: compile the daemon and the CLI into ./bin
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/wuxing ./cmd/wuxing
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/wxg     ./cmd/wxg

## test: unit tests with the race detector and coverage (short — no Docker)
test:
	$(GO) test ./... -short -race -cover

## test-integration: integration tests (requires a running Docker daemon)
test-integration:
	$(GO) test ./test/integration/... -race

## lint: run golangci-lint with the repo config
lint:
	$(GOLANGCI_LINT) run

## fmt: format the codebase
fmt:
	$(GO) run mvdan.cc/gofumpt@latest -w .

## run: build then start the kernel daemon
run: build
	$(BIN_DIR)/wuxing --manifest manifest/boot.yml

## tidy: sync go.mod / go.sum
tidy:
	$(GO) mod tidy

## docker: build the daemon image
docker:
	docker build -t wuxing:dev .

## clean: remove build output
clean:
	rm -rf $(BIN_DIR)
