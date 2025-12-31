# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./cmd/worker

# Server image
FROM alpine:3.19 AS server

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /server /app/server

EXPOSE 8080

ENTRYPOINT ["/app/server"]
CMD ["-addr", ":8080"]

# Worker image
FROM alpine:3.19 AS worker

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /worker /app/worker

ENTRYPOINT ["/app/worker"]
CMD ["-temporal-addr", "temporal:7233"]
