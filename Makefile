.PHONY: all build clean test test-integration fmt vet lint deps dev install coverage coverage-html help

BINARY_NAME = pgspectre
BIN_DIR     = bin
CMD_PATH    = ./cmd/pgspectre
VERSION    ?= dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     = -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE) -s -w"

## all: Run full CI pipeline
all: clean deps fmt vet test build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@rm -rf dist/
	@go clean
	@echo "Clean complete"

## test: Run tests with race detection and coverage
test:
	@echo "Running tests..."
	@go test -v -race -cover ./...

## test-integration: Run integration tests (requires Docker)
test-integration:
	@echo "Running integration tests..."
	@go test -race -tags=integration -count=1 -timeout=120s ./internal/...

## coverage: Run tests with coverage report
coverage:
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out

## coverage-html: Open coverage report in browser
coverage-html: coverage
	@go tool cover -html=coverage.out

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

## lint: Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run

## deps: Download and tidy dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

## dev: Run in development mode
dev:
	@go run $(CMD_PATH) $(ARGS)

## install: Install to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	@go install $(LDFLAGS) $(CMD_PATH)
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

## help: Show this help
help:
	@echo "pgspectre Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' Makefile
