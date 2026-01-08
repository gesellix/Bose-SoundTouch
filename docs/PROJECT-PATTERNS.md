# Project Structure Patterns: Bose SoundTouch API Client
## Summary for Reuse in API Client Projects

This document describes the most important patterns for the Bose SoundTouch API client, especially for XML-based API clients with Web UI, CLI tool, and WASM support.

## 1. Multi-Target Build Pattern

### The Core Pattern for Different Deployment Targets

```go
//go:build !wasm
// +build !wasm

// Native Go implementation

//go:build wasm
// +build wasm

// WASM-specific implementation
```

**Key Aspects:**
- **Native Builds**: Full API functionality for CLI and server
- **WASM Builds**: Browser-compatible subset functionality  
- **Cross-Platform**: Linux, macOS, Windows support
- **Embedded Assets**: Web UI directly embedded in binary

### Build System for Multi-Target

```makefile
# Native builds
build:
	go build -o $(BINARY_NAME) ./cmd/cli

# WASM build
build-wasm:
	GOOS=js GOARCH=wasm go build -o web/soundtouch.wasm ./cmd/wasm

# Web application with embedded assets
build-webapp:
	go build -o $(BINARY_NAME)-webapp ./cmd/webapp
```

## 2. XML-API Client Pattern

### HTTP Client with XML Parsing

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
    timeout    time.Duration
}

func NewClient(host string, port int) *Client {
    return &Client{
        baseURL:    fmt.Sprintf("http://%s:%d", host, port),
        httpClient: &http.Client{Timeout: 10 * time.Second},
        timeout:    10 * time.Second,
    }
}

func (c *Client) GetNowPlaying() (*models.NowPlaying, error) {
    resp, err := c.httpClient.Get(c.baseURL + "/now_playing")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var nowPlaying models.NowPlaying
    err = xml.NewDecoder(resp.Body).Decode(&nowPlaying)
    return &nowPlaying, err
}
```

**XML Request Pattern:**
```go
func (c *Client) SendKey(key models.Key) error {
    keyXML := fmt.Sprintf(`<key state="press" sender="GoClient">%s</key>`, key)
    
    resp, err := c.httpClient.Post(
        c.baseURL+"/key",
        "application/xml",
        strings.NewReader(keyXML),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

## 3. Device Discovery Pattern

### UPnP Discovery for Local Devices

```go
type DiscoveryService struct {
    timeout time.Duration
    cache   map[string]*Device
    mutex   sync.RWMutex
}

type Device struct {
    Name     string `json:"name"`
    Host     string `json:"host"`
    Port     int    `json:"port"`
    ModelID  string `json:"modelId"`
    SerialNo string `json:"serialNo"`
}

func (d *DiscoveryService) DiscoverDevices() ([]Device, error) {
    // UPnP SSDP Discovery implementation
    conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
    if err != nil {
        return nil, err
    }
    defer conn.Close()
    
    // Send M-SEARCH request
    searchRequest := "M-SEARCH * HTTP/1.1\r\n" +
        "HOST: 239.255.255.250:1900\r\n" +
        "MAN: \"ssdp:discover\"\r\n" +
        "ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
        "MX: 3\r\n\r\n"
    
    // Implementation details...
    return devices, nil
}
```

## 4. WebSocket Event Stream Pattern

### Real-time Updates for Audio Devices

```go
type EventClient struct {
    client     *Client
    conn       *websocket.Conn
    handlers   map[string]EventHandler
    stopChan   chan bool
    reconnect  bool
}

type EventHandler func(event Event)

type Event struct {
    Type      string      `xml:"type,attr"`
    DeviceID  string      `xml:"deviceID,attr"`
    Data      interface{} `xml:",innerxml"`
    Timestamp time.Time
}

func (e *EventClient) Subscribe(eventType string, handler EventHandler) {
    e.handlers[eventType] = handler
}

func (e *EventClient) Start() error {
    u := url.URL{Scheme: "ws", Host: e.client.host + ":8090", Path: "/"}
    
    conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
    if err != nil {
        return err
    }
    e.conn = conn
    
    go e.eventLoop()
    return nil
}

func (e *EventClient) eventLoop() {
    for {
        select {
        case <-e.stopChan:
            return
        default:
            var event Event
            err := e.conn.ReadJSON(&event)
            if err != nil {
                if e.reconnect {
                    e.reconnectWithBackoff()
                    continue
                }
                return
            }
            
            if handler, exists := e.handlers[event.Type]; exists {
                go handler(event)
            }
        }
    }
}
```

## 5. WASM JavaScript Bridge Pattern

### Go-to-JavaScript Function Mapping

```go
//go:build wasm
// +build wasm

import (
    "syscall/js"
    "encoding/json"
)

func RegisterWASMFunctions() {
    js.Global().Set("boseAPI", js.ValueOf(map[string]interface{}{
        "discoverDevices": js.FuncOf(wasmDiscoverDevices),
        "createClient":    js.FuncOf(wasmCreateClient),
        "getNowPlaying":   js.FuncOf(wasmGetNowPlaying),
        "sendKey":         js.FuncOf(wasmSendKey),
        "setVolume":       js.FuncOf(wasmSetVolume),
    }))
}

func wasmDiscoverDevices(this js.Value, args []js.Value) interface{} {
    handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        go func() {
            devices, err := discovery.NewDiscoveryService(5*time.Second).DiscoverDevices()
            
            result := make(map[string]interface{})
            if err != nil {
                result["error"] = err.Error()
            } else {
                devicesJSON, _ := json.Marshal(devices)
                result["devices"] = string(devicesJSON)
            }
            
            // Call JavaScript callback
            args[0].Invoke(js.ValueOf(result))
        }()
        return nil
    })
    
    return handler
}
```

### JavaScript Integration

```javascript
// Browser usage
async function discoverDevices() {
    return new Promise((resolve, reject) => {
        window.boseAPI.discoverDevices((result) => {
            if (result.error) {
                reject(new Error(result.error));
            } else {
                resolve(JSON.parse(result.devices));
            }
        });
    });
}

// Usage example
const devices = await discoverDevices();
const client = boseAPI.createClient(devices[0].host, 8090);
const nowPlaying = await client.getNowPlaying();
```

## 6. CLI Tool Pattern with Device Selection

### Interactive Device Selection

```go
// cmd/cli/main.go
func main() {
    app := &cli.App{
        Name:  "soundtouch",
        Usage: "Bose SoundTouch API Client",
        Commands: []*cli.Command{
            {
                Name:  "discover",
                Usage: "Discover SoundTouch devices",
                Action: func(c *cli.Context) error {
                    devices, err := discovery.DiscoverDevices()
                    if err != nil {
                        return err
                    }
                    
                    for i, device := range devices {
                        fmt.Printf("%d: %s (%s)\n", i+1, device.Name, device.Host)
                    }
                    return nil
                },
            },
            {
                Name:  "play",
                Usage: "Send play command",
                Flags: []cli.Flag{
                    &cli.StringFlag{Name: "device", Aliases: []string{"d"}},
                },
                Action: func(c *cli.Context) error {
                    client := getClientFromContext(c)
                    return client.SendKey(models.KeyPlay)
                },
            },
        },
    }
    
    app.Run(os.Args)
}

func getClientFromContext(c *cli.Context) *client.Client {
    deviceHost := c.String("device")
    if deviceHost == "" {
        // Interactive device selection
        devices, _ := discovery.DiscoverDevices()
        deviceHost = selectDeviceInteractive(devices)
    }
    
    return client.NewClient(deviceHost, 8090)
}
```

## 7. Web Application with Embedded Assets

### Single Binary Web Tool

```go
// cmd/webapp/main.go
//go:embed web
var webAssets embed.FS

func main() {
    mux := http.NewServeMux()
    
    // Embedded web assets
    webFS, err := fs.Sub(webAssets, "web")
    if err != nil {
        log.Fatal(err)
    }
    
    // SPA routing
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.FileServer(http.FS(webFS)).ServeHTTP(w, r)
            return
        }
        
        data, err := webAssets.ReadFile("web/index.html")
        if err != nil {
            http.Error(w, "Not found", http.StatusNotFound)
            return
        }
        
        w.Header().Set("Content-Type", "text/html")
        w.Write(data)
    })
    
    // API endpoints
    mux.HandleFunc("/api/devices", handleDeviceDiscovery)
    mux.HandleFunc("/api/client/", handleClientProxy)
    
    log.Println("SoundTouch Web UI starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### CORS Proxy for Browser Restrictions

```go
func handleClientProxy(w http.ResponseWriter, r *http.Request) {
    // Extract device IP from path: /api/client/192.168.1.100/now_playing
    pathParts := strings.Split(r.URL.Path, "/")
    if len(pathParts) < 5 {
        http.Error(w, "Invalid path", http.StatusBadRequest)
        return
    }
    
    deviceIP := pathParts[3]
    apiPath := "/" + strings.Join(pathParts[4:], "/")
    
    // Proxy request to SoundTouch device
    targetURL := fmt.Sprintf("http://%s:8090%s", deviceIP, apiPath)
    
    proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Copy headers
    for k, v := range r.Header {
        proxyReq.Header[k] = v
    }
    
    resp, err := http.DefaultClient.Do(proxyReq)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()
    
    // Enable CORS
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
    
    // Copy response
    w.WriteHeader(resp.StatusCode)
    io.Copy(w, resp.Body)
}
```

## 8. Robust XML Model Definition

### Structured Data Models

```go
// pkg/models/nowplaying.go
type NowPlaying struct {
    XMLName    xml.Name    `xml:"nowPlaying"`
    DeviceID   string      `xml:"deviceID,attr"`
    Source     string      `xml:"source,attr"`
    Content    ContentItem `xml:"ContentItem"`
    Track      string      `xml:"track"`
    Artist     string      `xml:"artist"`
    Album      string      `xml:"album"`
    Art        Art         `xml:"art"`
    PlayStatus PlayStatus  `xml:"playStatus"`
    Position   Position    `xml:"position,omitempty"`
}

type ContentItem struct {
    Source        string `xml:"source,attr"`
    Type          string `xml:"type,attr"`
    Location      string `xml:"location,attr"`
    SourceAccount string `xml:"sourceAccount,attr"`
    ItemName      string `xml:"itemName"`
    ContainerArt  string `xml:"containerArt"`
}

type PlayStatus string

const (
    PlayStatusPlaying PlayStatus = "PLAY_STATE"
    PlayStatusPaused  PlayStatus = "PAUSE_STATE"
    PlayStatusStopped PlayStatus = "STOP_STATE"
)

// Custom unmarshaling for enum validation
func (p *PlayStatus) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
    var s string
    if err := d.DecodeElement(&s, &start); err != nil {
        return err
    }
    
    switch s {
    case string(PlayStatusPlaying), string(PlayStatusPaused), string(PlayStatusStopped):
        *p = PlayStatus(s)
    default:
        *p = PlayStatusStopped // Default fallback
    }
    return nil
}
```

## 9. Configuration Management for Multi-Environment

### Environment-based Configuration

```go
// pkg/config/config.go
type Config struct {
    // Server configuration
    WebPort      int           `env:"WEB_PORT" default:"8080"`
    APITimeout   time.Duration `env:"API_TIMEOUT" default:"10s"`
    
    // Discovery configuration
    DiscoveryTimeout time.Duration `env:"DISCOVERY_TIMEOUT" default:"5s"`
    CacheDevices     bool          `env:"CACHE_DEVICES" default:"true"`
    
    // CORS configuration (for web proxy)
    CORSOrigins []string `env:"CORS_ORIGINS" default:"*"`
    
    // Logging
    LogLevel string `env:"LOG_LEVEL" default:"info"`
}

func Load() Config {
    var cfg Config
    
    // Load from .env file
    loadDotEnv()
    
    // Parse environment variables with reflection
    parseEnvVars(&cfg)
    
    return cfg
}

func parseEnvVars(cfg interface{}) {
    v := reflect.ValueOf(cfg).Elem()
    t := v.Type()
    
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)
        
        envTag := fieldType.Tag.Get("env")
        defaultTag := fieldType.Tag.Get("default")
        
        if envTag != "" {
            if envValue := os.Getenv(envTag); envValue != "" {
                setFieldValue(field, envValue)
            } else if defaultTag != "" {
                setFieldValue(field, defaultTag)
            }
        }
    }
}
```

## 10. Testing Pattern for Hardware API

### Mock-based Unit Tests

```go
// internal/testing/mock_client.go
type MockClient struct {
    responses map[string]interface{}
    errors    map[string]error
}

func NewMockClient() *MockClient {
    return &MockClient{
        responses: make(map[string]interface{}),
        errors:    make(map[string]error),
    }
}

func (m *MockClient) SetResponse(endpoint string, response interface{}) {
    m.responses[endpoint] = response
}

func (m *MockClient) SetError(endpoint string, err error) {
    m.errors[endpoint] = err
}

func (m *MockClient) GetNowPlaying() (*models.NowPlaying, error) {
    if err, exists := m.errors["now_playing"]; exists {
        return nil, err
    }
    
    if resp, exists := m.responses["now_playing"]; exists {
        return resp.(*models.NowPlaying), nil
    }
    
    return &models.NowPlaying{
        Track:  "Mock Track",
        Artist: "Mock Artist",
        Album:  "Mock Album",
    }, nil
}
```

### Integration Tests with Docker

```dockerfile
# test/docker/Dockerfile
FROM golang:1.21-alpine

WORKDIR /app
COPY . .

# Install test dependencies
RUN go mod download

# Run tests
CMD ["go", "test", "-v", "./..."]
```

```bash
# Makefile test target
test-integration:
	docker-compose -f test/docker-compose.yml up --build --abort-on-container-exit
	docker-compose -f test/docker-compose.yml down
```

## Recommended Project Structure

```
bose-soundtouch/
├── cmd/
│   ├── cli/              # CLI Tool
│   │   └── main.go
│   ├── webapp/           # Web Application
│   │   ├── main.go
│   │   └── web/          # Embedded Assets
│   │       ├── index.html
│   │       ├── app.js
│   │       └── style.css
│   └── wasm/             # WASM Entry Point
│       └── main.go
├── pkg/                  # Public API
│   ├── client/           # HTTP Client
│   ├── discovery/        # Device Discovery
│   ├── models/           # XML Data Models
│   ├── websocket/        # Event Streaming
│   └── wasm/             # WASM Bindings
├── internal/             # Private Implementation
│   ├── xml/              # XML Utilities
│   ├── http/             # HTTP Utilities
│   └── testing/          # Test Utilities
├── web/                  # Frontend Assets (source)
│   ├── src/
│   └── dist/             # Built assets → cmd/webapp/web/
├── examples/             # Usage Examples
├── test/                 # Integration Tests
├── Makefile              # Build Automation
├── .env.example          # Configuration Template
├── go.mod
└── README.md
```

## Build System for Multi-Target

### Makefile with Cross-Platform Support

```makefile
BINARY_NAME=soundtouch
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION=$(shell go version | cut -d ' ' -f 3)

LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION)"
BUILD_FLAGS=-trimpath $(LDFLAGS)

# Standard builds
build:
	go build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd/cli

build-webapp:
	go build $(BUILD_FLAGS) -o $(BINARY_NAME)-webapp ./cmd/webapp

# WASM build
build-wasm:
	GOOS=js GOARCH=wasm go build $(BUILD_FLAGS) -o web/soundtouch.wasm ./cmd/wasm
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" web/

# Cross-platform builds
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd/cli

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd/cli
	GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-arm64 ./cmd/cli

build-windows:
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-windows-amd64.exe ./cmd/cli

# Development
dev-webapp:
	air -c .air-webapp.toml

dev-wasm:
	GOOS=js GOARCH=wasm go build -o web/soundtouch.wasm ./cmd/wasm
	cd web && python3 -m http.server 8080

# Testing
test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Linting and formatting
check: fmt vet lint test

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Cleanup
clean:
	rm -f $(BINARY_NAME)*
	rm -f web/soundtouch.wasm web/wasm_exec.js
	rm -f coverage.out coverage.html

.PHONY: build build-webapp build-wasm build-all dev-webapp dev-wasm test check clean
```

## Reusable Patterns for API Client Projects

### 1. Basic Setup for XML-API Client

**Step 1:** Create project structure
```bash
mkdir -p cmd/{cli,webapp/web,wasm}
mkdir -p pkg/{client,discovery,models,websocket,wasm}
mkdir -p internal/{xml,http,testing}
mkdir -p examples test web/src
```

**Step 2:** Initialize Go module
```bash
go mod init github.com/username/api-client
go get github.com/gorilla/websocket
go get github.com/urfave/cli/v2
```

**Step 3:** Create basic HTTP client
```go
// pkg/client/client.go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func NewClient(baseURL string) *Client {
    return &Client{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}
```

### 2. XML Models Pattern

```go
// pkg/models/base.go
type XMLResponse struct {
    XMLName xml.Name `xml:",innerxml"`
    Error   *APIError `xml:"error,omitempty"`
}

type APIError struct {
    Code    string `xml:"code,attr"`
    Message string `xml:",innerxml"`
}

// pkg/models/device.go  
type DeviceInfo struct {
    XMLResponse
    Name     string `xml:"name"`
    Type     string `xml:"type"`
    DeviceID string `xml:"deviceID,attr"`
}
```

### 3. CLI Framework

```go
// cmd/cli/main.go
func main() {
    app := &cli.App{
        Name:    "api-client",
        Usage:   "API Client Tool",
        Version: Version,
        Commands: []*cli.Command{
            {
                Name:   "discover",
                Usage:  "Discover devices",
                Action: discoverCommand,
            },
            {
                Name:   "status",
                Usage:  "Get device status",
                Flags: deviceFlags,
                Action: statusCommand,
            },
        },
    }
    
    app.Run(os.Args)
}
```

## Advantages of This Pattern Approach

### 1. Multi-Platform Deployment
- **Native Binaries**: Optimal performance for server/CLI
- **WASM Support**: Browser integration without backend
- **Cross-Platform**: One codebase for all systems

### 2. API Client Best Practices
- **Type Safety**: Strict XML-to-Go mappings
- **Error Handling**: Structured error handling
- **Timeout Management**: Robust network calls

### 3. Developer Experience
- **Hot Reload**: Live updates during development
- **Mock Testing**: Hardware-independent testing
- **Comprehensive Tooling**: Build, test, lint automated

### 4. Production Ready
- **Graceful Shutdown**: Clean resource release
- **Structured Logging**: Monitoring-friendly logs
- **Configuration Management**: Environment-based config

## Conclusion

This pattern collection enables the development of robust API clients for hardware devices that function both as native tools and as web applications. The combination of Go's type safety, WASM support, and a structured build system makes it possible to use a single codebase for various deployment scenarios.