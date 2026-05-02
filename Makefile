.PHONY: install build run dev clean test lint

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Binary name
BINARY_NAME=quickcrawl
BINARY_SERVER=$(BINARY_NAME)-server
BINARY_MCP=$(BINARY_NAME)-mcp

# Main packages
SERVER_PKG=./cmd/server
MCP_PKG=./cmd/mcp

# Development
dev:
	air

# Install dependencies
install:
	$(GOMOD) download
	$(GOMOD) tidy

# Build binaries
build: build-server build-mcp

build-server:
	$(GOBUILD) -o bin/$(BINARY_SERVER) $(SERVER_PKG)

build-mcp:
	$(GOBUILD) -o bin/$(BINARY_MCP) $(MCP_PKG)

# Run binaries (production)
run-server: build-server
	./bin/$(BINARY_SERVER)

run-mcp: build-mcp
	./bin/$(BINARY_MCP)

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