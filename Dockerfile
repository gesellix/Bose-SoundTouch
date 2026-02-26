# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.0-alpine AS builder

ARG TARGETARCH=amd64
ARG TARGETOS=linux
ARG TARGETVARIANT=

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
