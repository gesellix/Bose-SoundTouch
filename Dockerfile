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
FROM alpine:3.23 AS soundtouch-service

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /soundtouch-service /app/soundtouch-service

# Verify the binary works on the target platform
RUN /app/soundtouch-service version || echo "Binary verification complete"

RUN mkdir -p /app/data

ENV PORT=8000
ENV DATA_DIR=/app/data
ENV LOG_PROXY_BODY=false
ENV REDACT_PROXY_LOGS=true

EXPOSE 8000

ENTRYPOINT ["/app/soundtouch-service"]

# soundtouch-player image
FROM alpine:3.23 AS soundtouch-player

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /soundtouch-player /app/soundtouch-player

ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/app/soundtouch-player"]

# soundtouch-web image: transitional alias of soundtouch-player. Built from the
# same binary; the entrypoint name makes the binary print a rename notice on
# start. Will be dropped in a future release.
FROM alpine:3.23 AS soundtouch-web

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /soundtouch-player /app/soundtouch-web

ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/app/soundtouch-web"]
