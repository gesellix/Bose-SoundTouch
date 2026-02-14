# Contributing to Bose SoundTouch API Client

Thank you for your interest in contributing to the Bose SoundTouch API Client! This project aims to provide a comprehensive, reliable, and well-tested Go library for controlling Bose SoundTouch devices.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Pull Request Process](#pull-request-process)
- [Coding Guidelines](#coding-guidelines)
- [Testing Guidelines](#testing-guidelines)
- [Documentation Guidelines](#documentation-guidelines)
- [Reporting Issues](#reporting-issues)
- [Device Testing](#device-testing)
- [Community](#community)

## Code of Conduct

This project adheres to our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

### Prerequisites

- **Go 1.25.6 or later**: [Download Go](https://golang.org/dl/)
- **Git**: For version control
- **Make**: For build automation (optional but recommended)
- **SoundTouch Device**: For testing (optional but valuable)

### First Contribution

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/Bose-SoundTouch.git
   cd Bose-SoundTouch
   ```
3. **Install dependencies**:
   ```bash
   go mod download
   ```
4. **Run tests** to ensure everything works:
   ```bash
   make test
   # or
   go test ./...
   ```
5. **Build the CLI** to test functionality:
   ```bash
   make build
   ./soundtouch-cli --help
   ```

## How Can I Contribute?

### 🐛 Reporting Bugs

Before creating a bug report, please:

1. **Check existing issues** to avoid duplicates
2. **Test with the latest version** from the main branch
3. **Include device information** (model, firmware version if known)

When filing a bug report, include:

- **Clear title** describing the issue
- **Steps to reproduce** the behavior
- **Expected behavior** vs actual behavior
- **Environment details**: OS, Go version, device model
- **Log output** if applicable (use `--verbose` flag)

### 💡 Suggesting Features

Feature requests are welcome! Please:

1. **Check if the feature already exists** in documentation
2. **Verify it's supported by the SoundTouch API** (see [official API docs](docs/reference/API-ENDPOINTS.md))
3. **Explain the use case** and how it benefits users

### 🔧 Contributing Code

Areas where contributions are especially welcome:

#### High Priority
- **Bug fixes** for existing functionality
- **Device compatibility** improvements
- **Error handling** enhancements
- **Performance optimizations**

#### Medium Priority
- **New endpoint implementations** (if officially documented)
- **CLI improvements** (better UX, additional commands)
- **Documentation improvements**
- **Example applications**

#### Future Enhancements
- **Web interface** development
- **Home Assistant integration**
- **WASM/browser support**
- **Mobile app development**

## Development Setup

### Project Structure

```
Bose-SoundTouch/
├── cmd/                    # Command-line applications
│   ├── soundtouch-cli/    # Main CLI tool
│   └── examples/          # Example applications
├── pkg/                   # Library packages
│   ├── client/           # HTTP client implementation
│   ├── discovery/        # Device discovery
│   ├── models/           # Data structures
│   └── config/           # Configuration management
├── docs/                 # Documentation
├── examples/             # Usage examples
└── scripts/              # Build and utility scripts
```

### Development Commands

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Build all binaries
make build

# Run linting and formatting
make check

# Run golangci-lint specifically
golangci-lint run

# Auto-fix linting issues where possible
golangci-lint run --fix

# Install CLI locally
go install ./cmd/soundtouch-cli

# Run integration tests (requires real device)
make test-integration HOST=192.168.1.100
```

### Environment Setup

For development with real devices, create a `.env` file:

```env
# Optional: Pre-configured device for testing
SOUNDTOUCH_HOST=192.168.1.100
SOUNDTOUCH_PORT=8090

# Optional: Enable debug logging
SOUNDTOUCH_DEBUG=true
```

## Pull Request Process

### Before Submitting

1. **Create an issue** first for significant changes
2. **Fork and create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. **Write tests** for your changes
4. **Update documentation** if needed
5. **Run the full test suite**:
   ```bash
   make check
   make test
   ```

### Pull Request Guidelines

1. **Clear title** describing the change
2. **Detailed description** explaining:
   - What the change does
   - Why it's needed
   - How it was tested
   - Any breaking changes
3. **Link to related issues**
4. **Update CHANGELOG.md** if applicable
5. **Ensure CI passes**

### Review Process

- At least one maintainer will review your PR
- Feedback will be constructive and specific
- Address feedback in additional commits
- Once approved, a maintainer will merge your PR

## Coding Guidelines

### Go Style

Follow standard Go conventions:

- **gofmt** for formatting
- **golangci-lint** for comprehensive code quality checks
- **go vet** for static analysis
- **Effective Go** principles
- **Standard library patterns** where applicable

### Code Organization

```go
// Package-level documentation
package client

import (
    // Standard library first
    "context"
    "encoding/xml"
    
    // Third-party packages
    "github.com/gorilla/websocket"
    
    // Local packages
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

// Public API should be well-documented
// GetDeviceInfo retrieves comprehensive device information including
// model, capabilities, network status, and current configuration.
func (c *Client) GetDeviceInfo() (*models.DeviceInfo, error) {
    // Implementation
}
```

### Error Handling

- **Return errors** instead of panicking
- **Wrap errors** with context using `fmt.Errorf`
- **Create custom error types** for specific conditions
- **Validate inputs** and return helpful error messages

```go
// Good error handling example
func (c *Client) SetVolume(level int) error {
    if level < 0 || level > 100 {
        return fmt.Errorf("volume level %d out of range [0-100]", level)
    }
    
    if err := c.post("/volume", volumeXML); err != nil {
        return fmt.Errorf("failed to set volume to %d: %w", level, err)
    }
    
    return nil
}
```

### API Design

- **Consistent method naming**: `Get*`, `Set*`, `Send*`, etc.
- **Return pointers** for complex types, values for simple types
- **Accept contexts** for potentially long-running operations
- **Provide convenience methods** for common operations

## Testing Guidelines

### Test Structure

```go
func TestClient_SetVolume(t *testing.T) {
    tests := []struct {
        name          string
        volume        int
        expectedError string
        setupMock     func(*httptest.Server)
    }{
        {
            name:   "valid volume level",
            volume: 50,
            setupMock: func(server *httptest.Server) {
                // Mock setup
            },
        },
        {
            name:          "volume too high",
            volume:        150,
            expectedError: "volume level 150 out of range",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Test Categories

1. **Unit Tests**: Test individual functions with mocks
2. **Integration Tests**: Test with real devices (when available)
3. **Benchmark Tests**: Performance testing for critical paths

### Mock Usage

Use `httptest.Server` for HTTP client testing:

```go
func setupMockServer() *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/info":
            w.Header().Set("Content-Type", "application/xml")
            fmt.Fprint(w, mockDeviceInfoXML)
        default:
            w.WriteHeader(http.StatusNotFound)
        }
    }))
}
```

### Real Device Testing

When possible, test with real SoundTouch devices:

```bash
# Set device IP for integration tests
export SOUNDTOUCH_HOST=192.168.1.100
go test -tags integration ./pkg/client/
```

## Documentation Guidelines

### Code Documentation

- **Package documentation** for every package
- **Function documentation** for all public functions
- **Example documentation** for complex usage

```go
// Package client provides a comprehensive HTTP client for the Bose SoundTouch Web API.
//
// The client supports all documented SoundTouch endpoints including device information,
// playback control, volume management, and real-time WebSocket events.
//
// Basic usage:
//
//     client := client.NewClient(&client.Config{
//         Host: "192.168.1.100",
//         Port: 8090,
//     })
//     
//     info, err := client.GetDeviceInfo()
//     if err != nil {
//         log.Fatal(err)
//     }
//     
//     fmt.Printf("Device: %s\n", info.Name)
package client
```

### User Documentation

- **README.md**: Overview and quick start
- **API documentation**: Comprehensive endpoint reference
- **Examples**: Real-world usage patterns
- **Troubleshooting**: Common issues and solutions

### Documentation Updates

When making changes:

1. **Update relevant docs** in the same PR
2. **Include usage examples** for new features
3. **Update CLI help text** if applicable
4. **Test documentation** (ensure examples work)

## Device Testing

### Supported Devices

The library has been tested with:

- **SoundTouch 10** (firmware unknown)
- **SoundTouch 20** (firmware unknown)

### Testing New Devices

If you have access to other SoundTouch models:

1. **Run discovery** to find devices:
   ```bash
   ./soundtouch-cli discover devices
   ```

2. **Test basic functionality**:
   ```bash
   ./soundtouch-cli -h 192.168.1.100 info get
   ./soundtouch-cli -h 192.168.1.100 now-playing get
   ```

3. **Report compatibility** in your PR or issue
4. **Include device information** from the info endpoint

### Testing Protocol

For significant changes:

1. **Test on multiple devices** if available
2. **Test error scenarios** (device offline, network issues)
3. **Test edge cases** (invalid inputs, boundary conditions)
4. **Document any device-specific behavior**

## Reporting Issues

### Security Issues

**Do not open public issues for security vulnerabilities.** Instead:

1. **Email the maintainers** with details
2. **Allow reasonable time** for response
3. **Coordinate disclosure** timing

### Bug Reports

Use the bug report template and include:

- **Device model and firmware** (if known)
- **Complete error messages and logs**
- **Minimal reproduction case**
- **Environment information**

### Feature Requests

Use the feature request template and include:

- **Clear description** of the desired functionality
- **Use case explanation** 
- **API documentation reference** (if applicable)
- **Alternative solutions** you've considered

## Community

### Communication Channels

- **GitHub Issues**: Bug reports, feature requests
- **GitHub Discussions**: Questions, ideas, general discussion
- **Pull Requests**: Code contributions and reviews

### Getting Help

1. **Check existing documentation** first
2. **Search closed issues** for similar problems
3. **Create a new issue** with detailed information
4. **Be patient and respectful** in all interactions

### Recognition

Contributors will be:

- **Listed in CONTRIBUTORS.md**
- **Mentioned in release notes** for significant contributions
- **Credited in documentation** where appropriate

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Bose SoundTouch API Documentation](docs/reference/API-ENDPOINTS.md)
- [Project Architecture](docs/PROJECT-PATTERNS.md)
- [Development Status](docs/archive/STATUS.md)

---

**Thank you for contributing!** Every contribution helps make this library better for the entire SoundTouch community.
