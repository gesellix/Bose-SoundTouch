.PHONY: all build build-cli test test-coverage check fmt vet lint clean dev help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build parameters
BINARY_NAME=soundtouch-cli
BINARY_PATH=./cmd/$(BINARY_NAME)
BUILD_DIR=./build

# Version info
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Linker flags
LDFLAGS=-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.Commit=$(COMMIT)

all: check build

build: build-cli

build-cli:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)

build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(BINARY_PATH)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(BINARY_PATH)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)

test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

check: fmt vet test

fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

dev: build-cli
	@echo "Starting development CLI..."
	$(BUILD_DIR)/$(BINARY_NAME) -help

dev-discover: build-cli
	@echo "Running device discovery..."
	$(BUILD_DIR)/$(BINARY_NAME) -discover

dev-info: build-cli
	@echo "Getting device info (requires -host flag)..."
	@if [ -z "$(HOST)" ]; then \
		echo "Usage: make dev-info HOST=192.168.1.100"; \
		exit 1; \
	fi
	$(BUILD_DIR)/$(BINARY_NAME) -host $(HOST) -info

install: build-cli
	@echo "Installing $(BINARY_NAME) to $(GOPATH)/bin..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

release: clean check build-all
	@echo "Creating release archive..."
	@mkdir -p $(BUILD_DIR)/release
	@for binary in $(BUILD_DIR)/$(BINARY_NAME)-*; do \
		if [ -f "$$binary" ]; then \
			cp "$$binary" $(BUILD_DIR)/release/; \
		fi \
	done
	@echo "Release binaries created in $(BUILD_DIR)/release/"

docker-build:
	@echo "Building Docker image..."
	docker build -t soundtouch-go:$(VERSION) .

docker-dev: docker-build
	@echo "Running development container..."
	docker run --rm -it --network host soundtouch-go:$(VERSION)

help:
	@echo "Available targets:"
	@echo "  build         - Build the CLI tool"
	@echo "  build-all     - Build for all platforms"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  check         - Run fmt, vet, and tests"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run golangci-lint"
	@echo "  tidy          - Tidy dependencies"
	@echo "  dev           - Build and show CLI help"
	@echo "  dev-discover  - Build and run device discovery"
	@echo "  dev-info      - Build and get device info (HOST=ip required)"
	@echo "  install       - Install binary to GOPATH/bin"
	@echo "  clean         - Clean build artifacts"
	@echo "  release       - Create release binaries"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-dev    - Run development container"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make dev-discover"
	@echo "  make dev-info HOST=192.168.1.100"
	@echo "  make test"
	@echo "  make build-all"
