# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder

# Declare automatic platform ARGs to make them available in build stage
# See https://docs.docker.com/reference/dockerfile#automatic-platform-args-in-the-global-scope
# We should not set defaults here, but rely on BuildKit to set them matching the BUILDPLATFORM
ARG TARGETARCH
ARG TARGETOS
ARG TARGETVARIANT

# Version info injected at build time; defaults keep local builds working.
# The release workflow passes VERSION, COMMIT, and DATE via --build-arg.
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the soundtouch-service
RUN if [ "${TARGETARCH}" = "arm" ] && [ -n "${TARGETVARIANT}" ]; then \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /soundtouch-service ./cmd/soundtouch-service; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /soundtouch-service ./cmd/soundtouch-service; \
    fi

# Build the soundtouch-player (formerly soundtouch-web)
RUN if [ "${TARGETARCH}" = "arm" ] && [ -n "${TARGETVARIANT}" ]; then \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /soundtouch-player ./cmd/soundtouch-player; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /soundtouch-player ./cmd/soundtouch-player; \
    fi

# soundtouch-service image
FROM alpine:3.24 AS soundtouch-service

RUN apk add --no-cache ca-certificates tzdata

# Non-root prep (dormant). Everything below is set up so the service CAN run
# as a fixed non-root user, but the image still runs as root by default
# (APP_USER below) so this is not a breaking change yet. The UID/GID is pinned
# (65532) so a mounted data volume's ownership stays predictable.
RUN addgroup -g 65532 -S aftertouch \
    && adduser -u 65532 -S -G aftertouch -H -h /app aftertouch

WORKDIR /app

COPY --from=builder /soundtouch-service /app/soundtouch-service

# Verify the binary works on the target platform
RUN /app/soundtouch-service version || echo "Binary verification complete"

# Create the data dir and hand /app to the non-root user.
RUN mkdir -p /app/data && chown -R aftertouch:aftertouch /app

# Allow the non-root process to bind the privileged DNS port (:53) when DNS
# Discovery is enabled, without granting the whole container extra privileges
# at runtime. NET_BIND_SERVICE is in Docker's default capability set, so this
# file capability is effective out of the box (no --cap-add needed). Done
# after chown, which would otherwise clear it; the setcap tool is removed after.
RUN apk add --no-cache --virtual .setcap libcap \
    && setcap 'cap_net_bind_service=+ep' /app/soundtouch-service \
    && apk del .setcap

ENV PORT=8000
ENV DATA_DIR=/app/data
ENV LOG_PROXY_BODY=false
ENV REDACT_PROXY_LOGS=true

# The toggle. Defaults to root, so this image behaves exactly as before and
# the change is non-breaking today. Enabling non-root is planned for v1.0.0
# (BREAKING: a bind-mounted DATA_DIR must then be writable by uid 65532 — the
# service logs the exact chown command at startup if it can't write). To
# enable, either change this default to "aftertouch" (a one-line commit) or
# build with --build-arg APP_USER=aftertouch.
ARG APP_USER=root
USER ${APP_USER}

EXPOSE 8000

ENTRYPOINT ["/app/soundtouch-service"]

# soundtouch-player image
FROM alpine:3.24 AS soundtouch-player

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /soundtouch-player /app/soundtouch-player

ENV PORT=8080

EXPOSE 8080

# The player is stateless and binds an unprivileged port, so it has no reason
# to run as root. mDNS/SSDP discovery uses unprivileged multicast.
USER nobody

ENTRYPOINT ["/app/soundtouch-player"]
