.PHONY: install build build-prod run dev clean test lint

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Binary names
BINARY_NAME=quickcrawl
BINARY_SERVER=$(BINARY_NAME)-server
BINARY_MCP=$(BINARY_NAME)-mcp

# Packages
SERVER_PKG=./cmd/server
MCP_PKG=./cmd/mcp
CLI_PKG=./cli

# Production ldflags: log level "error" suppresses info/warn but keeps errors visible.
# End users get zero noise; errors still print to stderr for debugging.
LDFLAGS_PROD=-s -w -X github.com/MabudAlam/quickcrawl/internal/utils.DefaultLevel=error

# Install dependencies
install:
	$(GOMOD) download
	$(GOMOD) tidy

# Development (info-level logging, same as before)
dev:
	air

# Build all three binaries (development, with logs)
build: build-cli build-server build-mcp

build-cli:
	$(GOBUILD) -o bin/$(BINARY_NAME) $(CLI_PKG)

build-server:
	$(GOBUILD) -o bin/$(BINARY_SERVER) $(SERVER_PKG)

build-mcp:
	$(GOBUILD) -o bin/$(BINARY_MCP) $(MCP_PKG)

# Build all three binaries for production distribution (silent by default)
build-prod: build-cli-prod build-server-prod build-mcp-prod

build-cli-prod:
	$(GOBUILD) -ldflags "$(LDFLAGS_PROD)" -o bin/$(BINARY_NAME) $(CLI_PKG)

build-server-prod:
	$(GOBUILD) -ldflags "$(LDFLAGS_PROD)" -o bin/$(BINARY_SERVER) $(SERVER_PKG)

build-mcp-prod:
	$(GOBUILD) -ldflags "$(LDFLAGS_PROD)" -o bin/$(BINARY_MCP) $(MCP_PKG)

# Run tests
test:
	$(GOTEST) -v -race ./...

# Run tests with coverage
test-cover:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Lint code
lint:
	golangci-lint run ./...

# Format code
fmt:
	$(GOFMT) ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Build and run server (no dev mode)
server: build-server
	./bin/$(BINARY_SERVER)