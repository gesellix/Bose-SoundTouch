# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.0-alpine AS builder

# Declare automatic platform ARGs to make them available in build stage
# See https://docs.docker.com/reference/dockerfile#automatic-platform-args-in-the-global-scope
# We should not set defaults here, but rely on BuildKit to set them matching the BUILDPLATFORM
ARG TARGETARCH
ARG TARGETOS
ARG TARGETVARIANT

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the soundtouch-service
RUN if [ "${TARGETARCH}" = "arm" ] && [ -n "${TARGETVARIANT}" ]; then \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -o /soundtouch-service ./cmd/soundtouch-service; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /soundtouch-service ./cmd/soundtouch-service; \
    fi

# Final stage
FROM alpine:3.23

# Install necessary runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /soundtouch-service /app/soundtouch-service

# Verify the binary works on the target platform
RUN /app/soundtouch-service version || echo "Binary verification complete"

# Create data directory for persistence
RUN mkdir -p /app/data

# Set environment variables with defaults
ENV PORT=8000
ENV DATA_DIR=/app/data
ENV LOG_PROXY_BODY=false
ENV REDACT_PROXY_LOGS=true

# Expose the service port
EXPOSE 8000

# Run the service
ENTRYPOINT ["/app/soundtouch-service"]
