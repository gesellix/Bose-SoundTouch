#!/bin/bash

# Bose SoundTouch Go Library - Release Preparation Script
# This script prepares everything needed for a GitHub release

set -e

VERSION=${1:-"v1.0.0"}
GITHUB_REPO="gesellix/bose-soundtouch"

echo "🚀 Preparing release $VERSION for $GITHUB_REPO"

# Verify we're in the right directory
if [ ! -f "go.mod" ] || [ ! -d "pkg/client" ]; then
    echo "❌ Error: Must be run from the project root directory"
    exit 1
fi

# Verify working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    echo "❌ Error: Working directory has uncommitted changes"
    git status --short
    exit 1
fi

# Verify we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "❌ Error: Must be on main branch (currently on $CURRENT_BRANCH)"
    exit 1
fi

# Run final tests
echo "🧪 Running tests..."
go test ./...
echo "✅ All tests passed"

# Build CLI for multiple platforms
echo "🔨 Building CLI for multiple platforms..."
mkdir -p build/releases

# Build for common platforms
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

for platform in "${PLATFORMS[@]}"; do
    IFS="/" read -r GOOS GOARCH <<< "$platform"
    OUTPUT_NAME="soundtouch-cli-$VERSION-$GOOS-$GOARCH"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="$OUTPUT_NAME.exe"
    fi

    echo "  Building for $GOOS/$GOARCH..."
    GOOS=$GOOS GOARCH=$GOARCH go build -o "build/releases/$OUTPUT_NAME" ./cmd/soundtouch-cli
done

echo "✅ Built CLI for all platforms"

# Generate checksums
echo "🔐 Generating checksums..."
cd build/releases
sha256sum * > checksums.sha256
cd ../..
echo "✅ Generated checksums"

# Create release notes
echo "📝 Generating release notes..."
cat > build/RELEASE_NOTES.md << EOF
# Bose SoundTouch Go Library $VERSION

A comprehensive Go library for controlling Bose SoundTouch speakers with 100% API coverage, real-time WebSocket events, and production-ready features.

## 🎯 Key Features

- **100% API Coverage**: All 19 official endpoints + 6 useful extensions (25 total)
- **Real-time Events**: WebSocket support with auto-reconnect and comprehensive event handling
- **Multiroom Control**: Complete zone management and coordination
- **Production Ready**: Connection pooling, error handling, circuit breakers, monitoring
- **Excellent Documentation**: 4000+ lines including Getting Started, Cookbook, Troubleshooting, and Deployment guides
- **CLI Tool**: Full-featured command-line interface with all endpoints

## 🚀 Quick Start

\`\`\`bash
go get github.com/$GITHUB_REPO
\`\`\`

\`\`\`go
package main

import (
    "fmt"
    "log"

    "github.com/$GITHUB_REPO/pkg/client"
)

func main() {
    // Create client
    c := client.New("192.168.1.100", 8090)

    // Get device info
    info, err := c.GetInfo()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Device: %s\\n", info.Name)
}
\`\`\`

## 📚 Documentation

- [Getting Started Guide](docs/GETTING-STARTED.md) - 10-minute tutorial from discovery to WebSocket monitoring
- [API Cookbook](docs/API-COOKBOOK.md) - 1000+ lines of real-world patterns and examples
- [Troubleshooting Guide](docs/TROUBLESHOOTING.md) - Systematic issue resolution
- [Deployment Guide](docs/DEPLOYMENT.md) - Production deployment examples (Docker, K8s, systemd)

## 🔧 CLI Tool

Download the CLI tool for your platform from the releases below:

\`\`\`bash
# Quick device discovery
./soundtouch-cli -discover

# Get device information
./soundtouch-cli -host 192.168.1.100 -info

# Monitor real-time events
./soundtouch-cli -host 192.168.1.100 -nowplaying
\`\`\`

## 🧪 Tested Hardware

- Bose SoundTouch 10
- Bose SoundTouch 20
- All core functionality validated on real devices

## 📈 What's New in $VERSION

- Complete 100% API coverage (19/19 official endpoints)
- Added final endpoints: \`POST /name\`, \`GET /bassCapabilities\`, \`GET /trackInfo\`
- Fixed WebSocket XML tag issue
- Comprehensive documentation suite
- Production-ready patterns and examples
- Real hardware validation

## 🤝 Contributing

Contributions welcome! See our documentation for examples and patterns.

## 📄 License

MIT License - see [LICENSE](LICENSE) file.

## 🙏 Acknowledgments

Built with real hardware testing and community feedback. Special thanks to the Bose developer community.
EOF

echo "✅ Generated release notes"

# Summary
echo ""
echo "🎉 Release preparation complete!"
echo ""
echo "📦 Files ready for release:"
ls -la build/releases/
echo ""
echo "📋 Next steps:"
echo "1. Push any remaining commits: git push origin main"
echo "2. Create and push tag: git tag $VERSION && git push origin $VERSION"
echo "3. Create GitHub release with files in build/releases/"
echo "4. Use build/RELEASE_NOTES.md as release description"
echo ""
echo "🔗 Release URL will be: https://github.com/$GITHUB_REPO/releases/tag/$VERSION"
EOF
