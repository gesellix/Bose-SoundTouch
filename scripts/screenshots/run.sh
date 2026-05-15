#!/usr/bin/env bash
# Orchestrates an end-to-end screenshot capture: spins up a clean
# soundtouch-service + dummy-speaker, drives the web UI in headless
# Chrome via the chromedp runner, then tears everything down.
#
# Outputs to docs/images/ by default. Override with OUT_DIR=/some/path.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUT_DIR="${OUT_DIR:-docs/images}"
SERVICE_PORT="${SERVICE_PORT:-8000}"
SPEAKER_PORT="${SPEAKER_PORT:-8090}"
DATA_DIR="$(mktemp -d -t soundtouch-screenshots-XXXXXX)"
LOG_DIR="$(mktemp -d -t soundtouch-screenshot-logs-XXXXXX)"

SERVICE_PID=""
SPEAKER_PID=""

cleanup() {
    set +e
    if [ -n "$SPEAKER_PID" ] && kill -0 "$SPEAKER_PID" 2>/dev/null; then
        kill "$SPEAKER_PID"
        wait "$SPEAKER_PID" 2>/dev/null
    fi
    if [ -n "$SERVICE_PID" ] && kill -0 "$SERVICE_PID" 2>/dev/null; then
        kill "$SERVICE_PID"
        wait "$SERVICE_PID" 2>/dev/null
    fi
    rm -rf "$DATA_DIR"
    echo "logs retained at $LOG_DIR"
}
trap cleanup EXIT

echo "==> building binaries"
go build -o "$LOG_DIR/soundtouch-service" ./cmd/soundtouch-service
go build -o "$LOG_DIR/dummy-speaker" ./cmd/dummy-speaker
go build -o "$LOG_DIR/screenshots" ./scripts/screenshots

echo "==> seeding settings.json (aftertouch.localhost + discovery off to avoid leaking real network info)"
# `aftertouch.localhost` is RFC 6761: any *.localhost name resolves to
# loopback via the system resolver in milliseconds (verified ~8ms on
# macOS / glibc / systemd-resolved). That gives us a brand-friendly URL
# in the screenshots without the ~5s DNS-timeout cliff that bites on
# unresolvable hostnames like aftertouch.local — that cliff compounds
# across /setup/settings + /setup/summary and pushes past the chromedp
# 30s per-shot budget.
cat > "$DATA_DIR/settings.json" <<'EOF'
{
  "server_url": "http://aftertouch.localhost:8000",
  "https_server_url": "https://aftertouch.localhost:8443",
  "discovery_enabled": false,
  "discovery_interval": "1h"
}
EOF

echo "==> starting soundtouch-service on :$SERVICE_PORT (data: $DATA_DIR)"
"$LOG_DIR/soundtouch-service" --port "$SERVICE_PORT" --data-dir "$DATA_DIR" \
    > "$LOG_DIR/service.log" 2>&1 &
SERVICE_PID=$!

echo "==> waiting for service to be ready"
for i in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:$SERVICE_PORT/setup/devices" > /dev/null 2>&1; then
        break
    fi
    if ! kill -0 "$SERVICE_PID" 2>/dev/null; then
        echo "service died early; log tail:"
        tail -40 "$LOG_DIR/service.log"
        exit 1
    fi
    sleep 0.5
done

echo "==> starting dummy-speaker on :$SPEAKER_PORT (registering with service)"
# Register as bare IP (no port) so the service appends :8090 for HTTP and
# :17000 for telnet exactly the way it does with real hardware. This is
# also why the listeners below bind to the canonical Bose ports.
"$LOG_DIR/dummy-speaker" \
    --listen "127.0.0.1:$SPEAKER_PORT" \
    --telnet-listen "127.0.0.1:17000" \
    --register "http://127.0.0.1:$SERVICE_PORT" \
    --register-as "127.0.0.1" \
    > "$LOG_DIR/speaker.log" 2>&1 &
SPEAKER_PID=$!
sleep 1

if ! kill -0 "$SPEAKER_PID" 2>/dev/null; then
    echo "dummy-speaker died early; log tail:"
    tail -40 "$LOG_DIR/speaker.log"
    exit 1
fi

echo "==> capturing screenshots into $OUT_DIR"
"$LOG_DIR/screenshots" \
    --base "http://127.0.0.1:$SERVICE_PORT" \
    --manifest scripts/screenshots/manifest.json \
    --out "$OUT_DIR"

echo "==> done"
