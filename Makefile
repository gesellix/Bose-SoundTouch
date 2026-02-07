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
SERVICE_NAME=soundtouch-service
SERVICE_PATH=./cmd/$(SERVICE_NAME)
EXAMPLE_MDNS_NAME=example-mdns
EXAMPLE_MDNS_PATH=./cmd/$(EXAMPLE_MDNS_NAME)
EXAMPLE_UPNP_NAME=example-upnp
EXAMPLE_UPNP_PATH=./cmd/$(EXAMPLE_UPNP_NAME)
SCANNER_NAME=mdns-scanner
SCANNER_PATH=./cmd/$(SCANNER_NAME)
BUILD_DIR=./build

# Version info
# No ldflags needed - using debug.BuildInfo since Go 1.18

all: check build

build: build-cli build-service build-examples

build-cli:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)

build-service:
	@echo "Building $(SERVICE_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME) $(SERVICE_PATH)

build-examples:
	@echo "Building $(EXAMPLE_MDNS_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_MDNS_NAME) $(EXAMPLE_MDNS_PATH)
	@echo "Building $(EXAMPLE_UPNP_NAME)..."
	$(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_UPNP_NAME) $(EXAMPLE_UPNP_PATH)
	@echo "Building $(SCANNER_NAME)..."
	$(GOBUILD) -o $(BUILD_DIR)/$(SCANNER_NAME) $(SCANNER_PATH)

build-all: build-linux build-darwin build-windows build-examples-all

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(BINARY_PATH)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME)-linux-amd64 $(SERVICE_PATH)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(BINARY_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME)-darwin-amd64 $(SERVICE_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME)-darwin-arm64 $(SERVICE_PATH)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SERVICE_NAME)-windows-amd64.exe $(SERVICE_PATH)

build-examples-all:
	@echo "Building examples for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_MDNS_NAME)-linux-amd64 $(EXAMPLE_MDNS_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_MDNS_NAME)-darwin-amd64 $(EXAMPLE_MDNS_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_MDNS_NAME)-darwin-arm64 $(EXAMPLE_MDNS_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_MDNS_NAME)-windows-amd64.exe $(EXAMPLE_MDNS_PATH)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_UPNP_NAME)-linux-amd64 $(EXAMPLE_UPNP_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_UPNP_NAME)-darwin-amd64 $(EXAMPLE_UPNP_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_UPNP_NAME)-darwin-arm64 $(EXAMPLE_UPNP_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(EXAMPLE_UPNP_NAME)-windows-amd64.exe $(EXAMPLE_UPNP_PATH)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SCANNER_NAME)-linux-amd64 $(SCANNER_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SCANNER_NAME)-darwin-amd64 $(SCANNER_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(SCANNER_NAME)-darwin-arm64 $(SCANNER_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(SCANNER_NAME)-windows-amd64.exe $(SCANNER_PATH)

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

dev-service: build-service
	@echo "Starting development service..."
	$(BUILD_DIR)/$(SERVICE_NAME)

dev-service-proxy: build-service
	@echo "Starting development service with proxy..."
	@if [ -z "$(PROXY_URL)" ]; then \
		echo "Usage: make dev-service-proxy PROXY_URL=http://localhost:8001"; \
		exit 1; \
	fi
	PYTHON_BACKEND_URL=$(PROXY_URL) $(BUILD_DIR)/$(SERVICE_NAME)

dev-discover: build-cli
	@echo "Running device discovery..."
	$(BUILD_DIR)/$(BINARY_NAME) -discover

dev-info: build-cli
	@echo "Getting device info (requires -host flag)..."
	@if [ -z "$(HOST)" ]; then \
		echo "Usage: make dev-info HOST=192.168.1.10"; \
		exit 1; \
	fi
	$(BUILD_DIR)/$(BINARY_NAME) -host $(HOST) -info

dev-mdns: build-examples
	@echo "Running mDNS discovery example..."
	$(BUILD_DIR)/$(EXAMPLE_MDNS_NAME)

dev-mdns-verbose: build-examples
	@echo "Running mDNS discovery example with verbose logging..."
	$(BUILD_DIR)/$(EXAMPLE_MDNS_NAME) -v

dev-mdns-timeout: build-examples
	@echo "Running mDNS discovery example with custom timeout..."
	@if [ -z "$(TIMEOUT)" ]; then \
		echo "Usage: make dev-mdns-timeout TIMEOUT=10s"; \
		exit 1; \
	fi
	$(BUILD_DIR)/$(EXAMPLE_MDNS_NAME) -timeout $(TIMEOUT) -v

dev-upnp: build-examples
	@echo "Running UPnP/SSDP discovery example..."
	$(BUILD_DIR)/$(EXAMPLE_UPNP_NAME)

dev-upnp-verbose: build-examples
	@echo "Running UPnP/SSDP discovery example with verbose logging..."
	$(BUILD_DIR)/$(EXAMPLE_UPNP_NAME) -v

dev-upnp-timeout: build-examples
	@echo "Running UPnP/SSDP discovery example with custom timeout..."
	@if [ -z "$(TIMEOUT)" ]; then \
		echo "Usage: make dev-upnp-timeout TIMEOUT=10s"; \
		exit 1; \
	fi
	$(BUILD_DIR)/$(EXAMPLE_UPNP_NAME) -timeout $(TIMEOUT) -v

dev-scan-all: build-examples
	@echo "Scanning all mDNS services on network..."
	$(BUILD_DIR)/$(SCANNER_NAME) -v

dev-scan-soundtouch: build-examples
	@echo "Scanning for SoundTouch mDNS services..."
	$(BUILD_DIR)/$(SCANNER_NAME) -service _soundtouch._tcp -v

dev-scan-http: build-examples
	@echo "Scanning for HTTP mDNS services..."
	$(BUILD_DIR)/$(SCANNER_NAME) -service _http._tcp -v

install: build-cli build-service
	@echo "Installing binaries to $(GOPATH)/bin..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	cp $(BUILD_DIR)/$(SERVICE_NAME) $(GOPATH)/bin/

clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

release: clean check build-all
	@echo "Creating release archive..."
	@mkdir -p $(BUILD_DIR)/release
	@for binary in $(BUILD_DIR)/$(BINARY_NAME)-* $(BUILD_DIR)/$(SERVICE_NAME)-*; do \
		if [ -f "$$binary" ]; then \
			cp "$$binary" $(BUILD_DIR)/release/; \
		fi \
	done
	@echo "Release binaries created in $(BUILD_DIR)/release/"

docker-build:
	@echo "Building Docker image..."
	docker build -t soundtouch-service .

docker-run-host:
	@echo "Running Docker container..."
	@echo "Note: --network host is used for discovery (Linux only). For macOS/Windows use port mapping."
	docker run --rm -it --network host -v $$(pwd)/data:/app/data soundtouch-service

docker-run-ports:
	@echo "Running Docker container with port mapping (discovery will be manual)..."
	docker run --rm -it -p 8000:8000 -v $$(pwd)/data:/app/data soundtouch-service

help:
	@echo "Available targets:"
	@echo "  build         - Build the CLI tool, service, and examples"
	@echo "  build-cli     - Build only the CLI tool"
	@echo "  build-service - Build only the service"
	@echo "  build-examples - Build only the example programs"
	@echo "  build-all     - Build for all platforms"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  check         - Run fmt, vet, and tests"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run golangci-lint"
	@echo "  tidy          - Tidy dependencies"
	@echo "  dev           - Build and show CLI help"
	@echo "  dev-service   - Build and run service locally"
	@echo "  dev-service-proxy - Build and run service with proxy (PROXY_URL=url required)"
	@echo "  dev-discover  - Build and run device discovery"
	@echo "  dev-info      - Build and get device info (HOST=ip required)"
	@echo "  dev-mdns      - Build and run mDNS discovery example"
	@echo "  dev-mdns-verbose - Build and run mDNS example with detailed logging"
	@echo "  dev-mdns-timeout - Build and run mDNS example with custom timeout (TIMEOUT=10s)"
	@echo "  dev-upnp      - Build and run UPnP/SSDP discovery example"
	@echo "  dev-upnp-verbose - Build and run UPnP example with detailed logging"
	@echo "  dev-upnp-timeout - Build and run UPnP example with custom timeout (TIMEOUT=10s)"
	@echo "  dev-scan-all     - Scan all mDNS services on network"
	@echo "  dev-scan-soundtouch - Scan specifically for SoundTouch mDNS services"
	@echo "  dev-scan-http    - Scan for HTTP mDNS services"
	@echo "  install       - Install binaries to GOPATH/bin"
	@echo "  clean         - Clean build artifacts"
	@echo "  release       - Create release binaries"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run-host  - Run container with host networking (Linux discovery)"
	@echo "  docker-run-ports - Run container with port mapping (macOS/Windows/No discovery)"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make dev-service"
	@echo "  make dev-service-proxy PROXY_URL=http://192.168.1.50:8001"
	@echo "  make dev-discover"
	@echo "  make dev-info HOST=192.168.1.10"
	@echo "  make dev-mdns"
	@echo "  make dev-mdns-verbose"
	@echo "  make dev-mdns-timeout TIMEOUT=10s"
	@echo "  make dev-upnp"
	@echo "  make dev-upnp-verbose"
	@echo "  make dev-upnp-timeout TIMEOUT=10s"
	@echo "  make dev-scan-all"
	@echo "  make dev-scan-soundtouch"
	@echo "  make test"
	@echo "  make build-all"
