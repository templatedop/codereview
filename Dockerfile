# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/bin/postgres-worker \
    ./cmd/postgres-worker

# Runtime stage
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/postgres-worker /app/postgres-worker

# Create data directory for vector store persistence
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports
# 8080 - HTTP server (webhooks, health checks)
# 9090 - Prometheus metrics
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/live || exit 1

# Set default environment variables
ENV ENVIRONMENT=production \
    LOG_FORMAT=json \
    LOG_LEVEL=info \
    SERVER_HOST=0.0.0.0 \
    SERVER_PORT=8080 \
    METRICS_PORT=9090 \
    VECTORSTORE_STORE_PATH=/app/data/vectorstore.json

# Run the binary
ENTRYPOINT ["/app/postgres-worker"]
