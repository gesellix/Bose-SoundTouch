# SoundTouch Production Deployment Guide

**Best practices for deploying SoundTouch Go applications in production environments**

This guide covers everything you need to know to deploy robust, scalable SoundTouch applications in production, including configuration management, monitoring, security, and operational considerations.

## 📋 **Table of Contents**

- [Architecture Considerations](#architecture-considerations)
- [Configuration Management](#configuration-management)
- [Security & Network](#security--network)
- [Monitoring & Logging](#monitoring--logging)
- [Performance Optimization](#performance-optimization)
- [Error Handling & Recovery](#error-handling--recovery)
- [Deployment Strategies](#deployment-strategies)
- [Maintenance & Operations](#maintenance--operations)

---

## 🏗️ **Architecture Considerations**

### Single-Device Applications

**Use Case**: Home automation, personal music control

```go
type SingleDeviceApp struct {
    client    *client.Client
    wsClient  *client.WebSocketClient
    config    Config
    logger    *log.Logger
    metrics   *Metrics
}

func NewSingleDeviceApp(config Config) *SingleDeviceApp {
    // Use resilient client with retries
    resilientClient := NewResilientClient(client.NewClient(client.ClientConfig{
        Host:    config.DeviceHost,
        Port:    config.DevicePort,
        Timeout: config.RequestTimeout,
    }))

    return &SingleDeviceApp{
        client: resilientClient,
        config: config,
        logger: log.New(os.Stdout, "[SoundTouch] ", log.LstdFlags),
    }
}
```

### Multi-Device Applications

**Use Case**: Commercial installations, whole-house systems

```go
type MultiDeviceManager struct {
    pool        *ConnectionPool
    devices     map[string]*DeviceInfo
    healthCheck *HealthChecker
    config      Config
    metrics     *prometheus.Registry
}

type DeviceInfo struct {
    Client      *client.Client
    Name        string
    Location    string
    Capabilities []string
    LastSeen    time.Time
    Status      DeviceStatus
}

func NewMultiDeviceManager(config Config) *MultiDeviceManager {
    return &MultiDeviceManager{
        pool:    NewConnectionPool(config.MaxConnections, config.IdleTimeout),
        devices: make(map[string]*DeviceInfo),
        config:  config,
    }
}
```

### Microservice Architecture

**Use Case**: Enterprise integrations, API services

```go
// Service interface for dependency injection
type SoundTouchService interface {
    GetDevices() ([]*DeviceInfo, error)
    ControlDevice(deviceID string, action Action) error
    GetDeviceStatus(deviceID string) (*Status, error)
}

// Implementation with circuit breakers, metrics, tracing
type ProductionSoundTouchService struct {
    manager     *MultiDeviceManager
    circuitBreaker *gobreaker.CircuitBreaker
    tracer      opentracing.Tracer
    metrics     metrics.Counter
}
```

---

## ⚙️ **Configuration Management**

### Environment-Based Configuration

```go
type Config struct {
    // Server settings
    ListenAddr      string        `env:"LISTEN_ADDR" default:":8080"`
    
    // SoundTouch settings
    DeviceHosts     []string      `env:"DEVICE_HOSTS" separator:","`
    DiscoveryTimeout time.Duration `env:"DISCOVERY_TIMEOUT" default:"30s"`
    RequestTimeout   time.Duration `env:"REQUEST_TIMEOUT" default:"15s"`
    MaxRetries       int           `env:"MAX_RETRIES" default:"3"`
    
    // Connection pool
    MaxConnections   int           `env:"MAX_CONNECTIONS" default:"10"`
    IdleTimeout      time.Duration `env:"IDLE_TIMEOUT" default:"5m"`
    
    // Monitoring
    MetricsEnabled   bool          `env:"METRICS_ENABLED" default:"true"`
    HealthCheckInterval time.Duration `env:"HEALTH_CHECK_INTERVAL" default:"30s"`
    
    // Logging
    LogLevel        string         `env:"LOG_LEVEL" default:"info"`
    LogFormat       string         `env:"LOG_FORMAT" default:"json"`
    
    // Security
    EnableTLS       bool          `env:"ENABLE_TLS" default:"false"`
    TLSCertFile     string        `env:"TLS_CERT_FILE"`
    TLSKeyFile      string        `env:"TLS_KEY_FILE"`
}

func LoadConfig() (*Config, error) {
    cfg := &Config{}
    if err := env.Parse(cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }
    
    return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
    if len(c.DeviceHosts) == 0 {
        return fmt.Errorf("at least one device host must be specified")
    }
    
    if c.RequestTimeout < time.Second {
        return fmt.Errorf("request timeout must be at least 1 second")
    }
    
    if c.EnableTLS && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
        return fmt.Errorf("TLS cert and key files required when TLS is enabled")
    }
    
    return nil
}
```

### Configuration File Support

```yaml
# config/production.yaml
server:
  listen_addr: ":8080"
  enable_tls: true
  tls_cert_file: "/etc/ssl/certs/app.crt"
  tls_key_file: "/etc/ssl/private/app.key"

soundtouch:
  device_hosts:
    - "192.168.1.100"
    - "192.168.1.101"
  discovery_timeout: "30s"
  request_timeout: "15s"
  max_retries: 3

pool:
  max_connections: 20
  idle_timeout: "10m"

monitoring:
  metrics_enabled: true
  health_check_interval: "30s"
  
logging:
  level: "info"
  format: "json"
```

```go
func LoadConfigFromFile(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    
    return &cfg, cfg.Validate()
}
```

---

## 🔒 **Security & Network**

### Network Security

```go
// Network configuration with security considerations
type SecureNetworkConfig struct {
    // Allowed source IP ranges
    AllowedCIDRs    []string
    
    // Rate limiting
    RateLimit       int
    RateLimitWindow time.Duration
    
    // TLS configuration
    TLSConfig       *tls.Config
    
    // Timeouts for security
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    IdleTimeout     time.Duration
}

func NewSecureServer(config SecureNetworkConfig) *http.Server {
    mux := http.NewServeMux()
    
    // Add middleware
    handler := applyMiddleware(mux,
        corsMiddleware(),
        rateLimitMiddleware(config.RateLimit, config.RateLimitWindow),
        ipWhitelistMiddleware(config.AllowedCIDRs),
        loggingMiddleware(),
        metricsMiddleware(),
    )
    
    return &http.Server{
        Handler:      handler,
        TLSConfig:    config.TLSConfig,
        ReadTimeout:  config.ReadTimeout,
        WriteTimeout: config.WriteTimeout,
        IdleTimeout:  config.IdleTimeout,
    }
}
```

### Input Validation

```go
type DeviceControlRequest struct {
    DeviceID string `json:"device_id" validate:"required,uuid"`
    Action   string `json:"action" validate:"required,oneof=play pause stop"`
    Volume   *int   `json:"volume,omitempty" validate:"omitempty,min=0,max=100"`
    Source   string `json:"source,omitempty" validate:"omitempty,oneof=SPOTIFY BLUETOOTH AUX"`
}

func (r *DeviceControlRequest) Validate() error {
    validate := validator.New()
    if err := validate.Struct(r); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    // Additional business logic validation
    if r.Action == "volume" && r.Volume == nil {
        return fmt.Errorf("volume value required for volume action")
    }
    
    return nil
}
```

### Secrets Management

```go
// Use environment variables or secret management systems
type SecretsConfig struct {
    APIKeys map[string]string `env:"API_KEYS"`
    TLSCert string            `env:"TLS_CERT_PATH"`
    TLSKey  string            `env:"TLS_KEY_PATH"`
}

// For Kubernetes
func loadSecretsFromK8s() (*SecretsConfig, error) {
    // Read from mounted secret volumes
    tlsCert, err := os.ReadFile("/etc/secrets/tls.crt")
    if err != nil {
        return nil, err
    }
    
    tlsKey, err := os.ReadFile("/etc/secrets/tls.key")
    if err != nil {
        return nil, err
    }
    
    return &SecretsConfig{
        TLSCert: string(tlsCert),
        TLSKey:  string(tlsKey),
    }, nil
}
```

---

## 📊 **Monitoring & Logging**

### Structured Logging

```go
import (
    "github.com/sirupsen/logrus"
    "github.com/prometheus/client_golang/prometheus"
)

type Logger struct {
    *logrus.Logger
    deviceID string
    component string
}

func NewLogger(level, format, component string) (*Logger, error) {
    logger := logrus.New()
    
    // Set level
    logLevel, err := logrus.ParseLevel(level)
    if err != nil {
        return nil, err
    }
    logger.SetLevel(logLevel)
    
    // Set format
    if format == "json" {
        logger.SetFormatter(&logrus.JSONFormatter{
            TimestampFormat: time.RFC3339,
        })
    }
    
    return &Logger{
        Logger:    logger,
        component: component,
    }, nil
}

func (l *Logger) WithDevice(deviceID string) *logrus.Entry {
    return l.WithFields(logrus.Fields{
        "component": l.component,
        "device_id": deviceID,
    })
}

func (l *Logger) WithError(err error) *logrus.Entry {
    return l.WithField("error", err.Error())
}
```

### Metrics Collection

```go
type Metrics struct {
    // Request metrics
    RequestsTotal     prometheus.CounterVec
    RequestDuration   prometheus.HistogramVec
    RequestsInFlight  prometheus.GaugeVec
    
    // Device metrics
    DevicesConnected  prometheus.Gauge
    DeviceHealth      prometheus.GaugeVec
    WebSocketConnections prometheus.Gauge
    
    // Error metrics
    ErrorsTotal       prometheus.CounterVec
    
    // Business metrics
    VolumeChanges     prometheus.CounterVec
    SourceChanges     prometheus.CounterVec
    ZoneOperations    prometheus.CounterVec
}

func NewMetrics() *Metrics {
    m := &Metrics{
        RequestsTotal: *prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "soundtouch_requests_total",
                Help: "Total number of requests processed",
            },
            []string{"method", "endpoint", "status"},
        ),
        
        RequestDuration: *prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "soundtouch_request_duration_seconds",
                Help: "Request duration in seconds",
                Buckets: prometheus.DefBuckets,
            },
            []string{"method", "endpoint"},
        ),
        
        DevicesConnected: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "soundtouch_devices_connected",
                Help: "Number of connected devices",
            },
        ),
        
        DeviceHealth: *prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "soundtouch_device_health",
                Help: "Device health status (1=healthy, 0=unhealthy)",
            },
            []string{"device_id", "device_name"},
        ),
    }
    
    // Register metrics
    prometheus.MustRegister(
        m.RequestsTotal,
        m.RequestDuration,
        m.DevicesConnected,
        m.DeviceHealth,
    )
    
    return m
}

func (m *Metrics) RecordRequest(method, endpoint string, duration time.Duration, status int) {
    m.RequestsTotal.WithLabelValues(method, endpoint, fmt.Sprintf("%d", status)).Inc()
    m.RequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}
```

### Health Checks

```go
type HealthChecker struct {
    manager     *MultiDeviceManager
    interval    time.Duration
    timeout     time.Duration
    metrics     *Metrics
    logger      *Logger
}

func (hc *HealthChecker) Start(ctx context.Context) {
    ticker := time.NewTicker(hc.interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            hc.checkAllDevices()
        }
    }
}

func (hc *HealthChecker) checkAllDevices() {
    var wg sync.WaitGroup
    
    for deviceID, device := range hc.manager.devices {
        wg.Add(1)
        go func(id string, dev *DeviceInfo) {
            defer wg.Done()
            hc.checkDevice(id, dev)
        }(deviceID, device)
    }
    
    wg.Wait()
}

func (hc *HealthChecker) checkDevice(deviceID string, device *DeviceInfo) {
    ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
    defer cancel()
    
    start := time.Now()
    err := device.Client.Ping()
    duration := time.Since(start)
    
    if err != nil {
        device.Status = DeviceStatusUnhealthy
        hc.metrics.DeviceHealth.WithLabelValues(deviceID, device.Name).Set(0)
        hc.logger.WithDevice(deviceID).WithError(err).Error("Device health check failed")
    } else {
        device.Status = DeviceStatusHealthy
        device.LastSeen = time.Now()
        hc.metrics.DeviceHealth.WithLabelValues(deviceID, device.Name).Set(1)
        hc.logger.WithDevice(deviceID).WithField("duration", duration).Debug("Device health check passed")
    }
}

// HTTP health endpoint
func (hc *HealthChecker) HealthHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        healthy := 0
        total := 0
        
        for _, device := range hc.manager.devices {
            total++
            if device.Status == DeviceStatusHealthy {
                healthy++
            }
        }
        
        status := map[string]interface{}{
            "status": "ok",
            "devices": map[string]interface{}{
                "total":   total,
                "healthy": healthy,
                "unhealthy": total - healthy,
            },
            "timestamp": time.Now().UTC(),
        }
        
        w.Header().Set("Content-Type", "application/json")
        
        if healthy < total {
            w.WriteHeader(http.StatusServiceUnavailable)
            status["status"] = "degraded"
        }
        
        json.NewEncoder(w).Encode(status)
    }
}
```

---

## 🚀 **Performance Optimization**

### Connection Pooling

```go
type ConnectionPool struct {
    clients       sync.Map
    maxIdle       int
    maxActive     int
    idleTimeout   time.Duration
    activeCount   int64
    metrics       *Metrics
    mu            sync.RWMutex
}

func NewConnectionPool(maxIdle, maxActive int, idleTimeout time.Duration) *ConnectionPool {
    cp := &ConnectionPool{
        maxIdle:     maxIdle,
        maxActive:   maxActive,
        idleTimeout: idleTimeout,
    }
    
    // Start cleanup goroutine
    go cp.cleanup()
    
    return cp
}

func (cp *ConnectionPool) Get(host string, port int) (*client.Client, error) {
    key := fmt.Sprintf("%s:%d", host, port)
    
    // Check if connection exists and is valid
    if val, ok := cp.clients.Load(key); ok {
        conn := val.(*pooledConnection)
        if time.Since(conn.lastUsed) < cp.idleTimeout {
            conn.lastUsed = time.Now()
            return conn.client, nil
        }
        // Connection expired, remove it
        cp.clients.Delete(key)
    }
    
    // Check active connection limit
    if atomic.LoadInt64(&cp.activeCount) >= int64(cp.maxActive) {
        return nil, fmt.Errorf("connection pool exhausted")
    }
    
    // Create new connection
    config := client.ClientConfig{
        Host:    host,
        Port:    port,
        Timeout: 15 * time.Second,
    }
    
    newClient := client.NewClient(config)
    
    // Test connection
    if err := newClient.Ping(); err != nil {
        return nil, fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
    }
    
    conn := &pooledConnection{
        client:   newClient,
        lastUsed: time.Now(),
        created:  time.Now(),
    }
    
    cp.clients.Store(key, conn)
    atomic.AddInt64(&cp.activeCount, 1)
    
    return newClient, nil
}

type pooledConnection struct {
    client   *client.Client
    lastUsed time.Time
    created  time.Time
}

func (cp *ConnectionPool) cleanup() {
    ticker := time.NewTicker(cp.idleTimeout / 2)
    defer ticker.Stop()
    
    for range ticker.C {
        now := time.Now()
        cp.clients.Range(func(key, val interface{}) bool {
            conn := val.(*pooledConnection)
            if now.Sub(conn.lastUsed) > cp.idleTimeout {
                cp.clients.Delete(key)
                atomic.AddInt64(&cp.activeCount, -1)
            }
            return true
        })
    }
}
```

### Caching Strategy

```go
type CacheManager struct {
    deviceInfoCache   *cache.Cache
    capabilitiesCache *cache.Cache
    volumeCache       *cache.Cache
}

func NewCacheManager() *CacheManager {
    return &CacheManager{
        // Device info rarely changes, cache for 1 hour
        deviceInfoCache: cache.New(1*time.Hour, 2*time.Hour),
        
        // Capabilities never change, cache for 24 hours
        capabilitiesCache: cache.New(24*time.Hour, 48*time.Hour),
        
        // Volume changes frequently, cache for 5 seconds
        volumeCache: cache.New(5*time.Second, 10*time.Second),
    }
}

func (cm *CacheManager) GetDeviceInfo(deviceID string, fetcher func() (*models.DeviceInfo, error)) (*models.DeviceInfo, error) {
    if cached, found := cm.deviceInfoCache.Get(deviceID); found {
        return cached.(*models.DeviceInfo), nil
    }
    
    info, err := fetcher()
    if err != nil {
        return nil, err
    }
    
    cm.deviceInfoCache.Set(deviceID, info, cache.DefaultExpiration)
    return info, nil
}
```

---

## 🛡️ **Error Handling & Recovery**

### Circuit Breaker Pattern

```go
import "github.com/sony/gobreaker"

type ResilientSoundTouchService struct {
    client  *client.Client
    cb      *gobreaker.CircuitBreaker
    metrics *Metrics
}

func NewResilientSoundTouchService(client *client.Client) *ResilientSoundTouchService {
    settings := gobreaker.Settings{
        Name:        "soundtouch",
        MaxRequests: 3,
        Interval:    10 * time.Second,
        Timeout:     30 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
            return counts.Requests >= 3 && failureRatio >= 0.6
        },
        OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
            log.Printf("Circuit breaker '%s' changed from '%s' to '%s'", name, from, to)
        },
    }
    
    return &ResilientSoundTouchService{
        client: client,
        cb:     gobreaker.NewCircuitBreaker(settings),
    }
}

func (r *ResilientSoundTouchService) SetVolume(deviceID string, volume int) error {
    result, err := r.cb.Execute(func() (interface{}, error) {
        return nil, r.client.SetVolume(volume)
    })
    
    if err != nil {
        r.metrics.ErrorsTotal.WithLabelValues("circuit_breaker", "volume").Inc()
        return err
    }
    
    return result.(error)
}
```

### Graceful Shutdown

```go
func (app *Application) Run(ctx context.Context) error {
    // Setup signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    // Start services
    g, ctx := errgroup.WithContext(ctx)
    
    // HTTP server
    server := &http.Server{
        Addr:    app.config.ListenAddr,
        Handler: app.handler,
    }
    
    g.Go(func() error {
        app.logger.Info("Starting HTTP server", "addr", app.config.ListenAddr)
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            return err
        }
        return nil
    })
    
    // Health checker
    g.Go(func() error {
        return app.healthChecker.Start(ctx)
    })
    
    // WebSocket manager
    g.Go(func() error {
        return app.wsManager.Start(ctx)
    })
    
    // Wait for shutdown signal
    go func() {
        <-sigChan
        app.logger.Info("Shutdown signal received")
        
        // Graceful shutdown with timeout
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        // Shutdown HTTP server
        if err := server.Shutdown(shutdownCtx); err != nil {
            app.logger.Error("HTTP server shutdown error", "error", err)
        }
        
        // Close WebSocket connections
        app.wsManager.Shutdown(shutdownCtx)
        
        // Close connection pool
        app.connectionPool.Close()
    }()
    
    return g.Wait()
}
```

---

## 🚢 **Deployment Strategies**

### Docker Deployment

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config ./config

EXPOSE 8080
CMD ["./main"]
```

```yaml
# docker-compose.yml
version: '3.8'

services:
  soundtouch-app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DEVICE_HOSTS=192.168.1.100,192.168.1.101
      - LOG_LEVEL=info
      - METRICS_ENABLED=true
    volumes:
      - ./config:/app/config:ro
      - ./logs:/app/logs
    networks:
      - soundtouch-net
    restart: unless-stopped
    
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    networks:
      - soundtouch-net
      
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-storage:/var/lib/grafana
    networks:
      - soundtouch-net

networks:
  soundtouch-net:

volumes:
  grafana-storage:
```

### Kubernetes Deployment

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: soundtouch-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: soundtouch-app
  template:
    metadata:
      labels:
        app: soundtouch-app
    spec:
      containers:
      - name: soundtouch-app
        image: your-repo/soundtouch-app:latest
        ports:
        - containerPort: 8080
        env:
        - name: DEVICE_HOSTS
          valueFrom:
            configMapKeyRef:
              name: soundtouch-config
              key: device_hosts
        - name: LOG_LEVEL
          value: "info"
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

---
apiVersion: v1
kind: Service
metadata:
  name: soundtouch-service
spec:
  selector:
    app: soundtouch-app
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: soundtouch-config
data:
  device_hosts: "192.168.1.100,192.168.1.101,192.168.1.102"
```

### Systemd Service

```ini
# /etc/systemd/system/soundtouch.service
[Unit]
Description=SoundTouch Control Service
After=network.target
Wants=network.target

[Service]
Type=simple
User=soundtouch
Group=soundtouch
WorkingDirectory=/opt/soundtouch
ExecStart=/opt/soundtouch/bin/soundtouch-app
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
Environment=DEVICE_HOSTS=192.168.1.100,192.168.1.101
Environment=LOG_LEVEL=info
Environment=CONFIG_FILE=/opt/soundtouch/config/production.yaml

# Security settings
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/soundtouch/logs
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

---

## 🔧 **Maintenance & Operations**

### Log Rotation

```bash
# /etc/logrotate.d/soundtouch
/opt/soundtouch/logs/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0644 soundtouch soundtouch
    postrotate
        /bin/kill -HUP `cat /var/run/soundtouch.pid 2>/dev/null` 2>/dev/null || true
    endscript
}
```

### Monitoring Alerts

```yaml
# Prometheus alerts
groups:
- name: soundtouch
  rules:
  - alert: DeviceUnhealthy
    expr: soundtouch_device_health == 0
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "SoundTouch device {{ $labels.device_name }} is unhealthy"
      description: "Device {{ $labels.device_id }} has been unhealthy for more than 2 minutes"
      
  - alert: HighErrorRate
    expr: rate(soundtouch_errors_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High error rate detected"
      description: "Error rate is {{ $value }} errors/second over the last 5 minutes"
      
  - alert: ServiceDown
    expr: up{job="soundtouch"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "SoundTouch service is down"
      description: "SoundTouch service has been down for more than 1 minute"
```

### Backup Strategy

```go
// Backup device configurations
func (m *Manager) BackupConfigurations() error {
    backup := ConfigBackup{
        Timestamp: time.Now(),
        Devices:   make(map[string]DeviceConfig),
    }
    
    for deviceID, device := range m.devices {
        config := DeviceConfig{}
        
        // Backup presets
        if presets, err := device.Client.GetPresets(); err == nil {
            config.Presets = presets
        }
        
        // Backup settings
        if volume, err := device.Client.GetVolume(); err == nil {
            config.Volume = volume.TargetVolume
        }
        
        if bass, err := device.Client.GetBass(); err == nil {
            config.Bass = bass.TargetBass
        }
        
        backup.Devices[deviceID] = config
    }
    
    // Save to file
    data, err := json.MarshalIndent(backup, "", "  ")
    if err != nil {
        return err
    }
    
    filename := fmt.Sprintf("backup_%s.json", time.Now().Format("2006-01-02_15-04-05"))
    return os.WriteFile(filepath.Join(m.config.BackupDir, filename), data, 0644)
}
```

### Performance Tuning

```go
// Tune Go runtime for production
func init() {
    // Set GOMAXPROCS based on container limits if not set
    if os.Getenv("GOMAXPROCS") == "" {
        if limit := getCgroupCPULimit(); limit > 0 {
            runtime.GOMAXPROCS(int(limit))
        }
    }
    
    // Set GC target percentage
    if os.Getenv("GOGC") == "" {
        debug.SetGCPerc