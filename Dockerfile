# Build stage
FROM golang:1.26.0-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the soundtouch-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /soundtouch-service ./cmd/soundtouch-service

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
