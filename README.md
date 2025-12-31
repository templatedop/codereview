# PostgreSQL Code Review System

An AI-powered code review system specialized for PostgreSQL analysis, using RAG-enhanced LLM analysis with Temporal workflow orchestration.

## Features

- **PostgreSQL-Focused Analysis**: Performance, security, and standards compliance checking for SQL code
- **RAG-Enhanced Reviews**: Retrieval-Augmented Generation with PostgreSQL best practices knowledge base
- **Temporal Workflows**: Durable, distributed workflow orchestration for reliable processing
- **GitLab Integration**: Automated MR review comments with inline and summary feedback
- **Distributed Tracing**: OpenTelemetry integration for observability
- **Production-Ready**: Graceful shutdown, health checks, metrics, and structured logging

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   GitLab MR     │────▶│    Temporal     │────▶│     Worker      │
│   Webhook       │     │    Server       │     │   (Activities)  │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                        ┌─────────────────┐              │
                        │  Vector Store   │◀─────────────┤
                        │  (RAG Context)  │              │
                        └─────────────────┘              │
                                                         ▼
┌─────────────────┐                            ┌─────────────────┐
│ OpenTelemetry   │◀───────────────────────────│   LLM Client    │
│ (Jaeger/OTLP)   │                            │ (Ollama/OpenAI) │
└─────────────────┘                            └─────────────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│   Prometheus    │     │  Health Checks  │
│    Metrics      │     │  /health/*      │
└─────────────────┘     └─────────────────┘
```

## Components

| Component | Description |
|-----------|-------------|
| `cmd/postgres-worker` | Temporal worker for PostgreSQL code review workflows |
| `internal/rag` | RAG system with chunking, embedding, and vector store |
| `internal/postgres` | PostgreSQL-specific analyzers and SQL extraction |
| `internal/workflow` | Temporal workflow and activity definitions |
| `internal/gitlab` | GitLab API client for MR review comments |
| `internal/tracing` | OpenTelemetry distributed tracing |
| `internal/metrics` | Prometheus metrics instrumentation |
| `internal/health` | Kubernetes-compatible health checks |
| `internal/shutdown` | Graceful shutdown with request draining |
| `internal/config` | Centralized configuration management |
| `internal/errors` | Error handling with circuit breaker and retry |
| `internal/atomicfile` | Atomic file operations for data integrity |

## Prerequisites

- **Go 1.21+**
- **Temporal Server** (for workflow orchestration)
- **Ollama** or **OpenAI API** (for LLM inference)

### Install Ollama

```bash
# Install Ollama (https://ollama.ai)
curl -fsSL https://ollama.ai/install.sh | sh

# Pull recommended models
ollama pull qwen2.5-coder:32b      # For code analysis
ollama pull nomic-embed-text        # For embeddings
```

### Install Temporal

```bash
# Using Docker Compose (recommended)
git clone https://github.com/temporalio/docker-compose.git
cd docker-compose
docker-compose up -d

# Or using single container
docker run -d --name temporal \
  -p 7233:7233 \
  temporalio/auto-setup:latest
```

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd code-reviewer

# Install dependencies
go mod download

# Build all binaries
go build -o bin/ ./cmd/...

# Run tests
go test ./...
```

## Quick Start

### 1. Start Infrastructure

```bash
# Start Temporal
docker-compose -f docker-compose.temporal.yml up -d

# Start Ollama
ollama serve

# (Optional) Start Jaeger for tracing
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

### 2. Start the Worker

```bash
# With default configuration
./bin/postgres-worker

# With custom configuration
LLM_MODEL=qwen2.5-coder:32b \
TEMPORAL_HOST=localhost:7233 \
GITLAB_ENABLED=true \
GITLAB_TOKEN=your-token \
./bin/postgres-worker
```

### 3. Trigger a Review

```bash
# Via Temporal CLI
temporal workflow start \
  --task-queue code-review-queue \
  --type CodeReviewWorkflow \
  --input '{"file_path": "path/to/file.go"}'
```

## Configuration

All configuration is managed through environment variables with sensible defaults.

### Core Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVICE_NAME` | Service identifier | `code-reviewer` |
| `ENVIRONMENT` | Environment (dev/staging/prod) | `development` |

### Server Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_HOST` | HTTP server bind address | `0.0.0.0` |
| `SERVER_PORT` | HTTP server port | `8080` |
| `SERVER_READ_TIMEOUT` | Request read timeout | `30s` |
| `SERVER_WRITE_TIMEOUT` | Response write timeout | `10m` |
| `SERVER_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout | `30s` |

### Temporal Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `TEMPORAL_HOST` | Temporal server address | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `default` |
| `TEMPORAL_TASK_QUEUE` | Task queue name | `code-review-queue` |
| `TEMPORAL_WORKFLOW_TIMEOUT` | Workflow execution timeout | `30m` |
| `TEMPORAL_ACTIVITY_TIMEOUT` | Activity execution timeout | `5m` |
| `TEMPORAL_RETRY_MAX_ATTEMPTS` | Maximum retry attempts | `3` |

### LLM Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_PROVIDER` | LLM provider (ollama/openai/anthropic) | `ollama` |
| `LLM_BASE_URL` | LLM API endpoint | `http://localhost:11434` |
| `LLM_API_KEY` | API key (for OpenAI/Anthropic) | - |
| `LLM_MODEL` | Model name | `qwen2.5-coder:32b` |
| `LLM_TIMEOUT` | Request timeout | `5m` |
| `LLM_MAX_TOKENS` | Maximum output tokens | `4096` |
| `LLM_TEMPERATURE` | Sampling temperature | `0.1` |
| `LLM_REQUESTS_PER_MINUTE` | Rate limit | `60` |

### Embedder Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `EMBEDDER_PROVIDER` | Embedding provider (ollama/openai) | `ollama` |
| `EMBEDDER_BASE_URL` | Embedder API endpoint | `http://localhost:11434` |
| `EMBEDDER_API_KEY` | API key (for OpenAI) | - |
| `EMBEDDER_MODEL` | Embedding model | `nomic-embed-text` |
| `EMBEDDER_DIMENSION` | Embedding dimension | `768` |
| `EMBEDDER_TIMEOUT` | Request timeout | `60s` |
| `EMBEDDER_BATCH_SIZE` | Batch size for embeddings | `32` |
| `EMBEDDER_CACHE_ENABLED` | Enable embedding cache | `true` |
| `EMBEDDER_CACHE_MAX_SIZE` | Maximum cache entries | `10000` |
| `EMBEDDER_CACHE_TTL` | Cache TTL | `24h` |
| `EMBEDDER_CACHE_STORE_PATH` | Cache persistence path | - |

### RAG Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `RAG_CHUNK_SIZE` | Document chunk size (chars) | `500` |
| `RAG_CHUNK_OVERLAP` | Chunk overlap (chars) | `50` |
| `RAG_MIN_CHUNK_SIZE` | Minimum chunk size | `100` |
| `RAG_DEFAULT_TOP_K` | Default number of results | `5` |
| `RAG_MIN_SIMILARITY` | Minimum similarity threshold | `0.5` |
| `RAG_MAX_CONTEXT_LENGTH` | Maximum context length | `8000` |

### Vector Store Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `VECTORSTORE_TYPE` | Store type (memory/milvus) | `memory` |
| `VECTORSTORE_STORE_PATH` | Persistence path | `./data/vectorstore.json` |
| `MILVUS_HOST` | Milvus server host | `localhost` |
| `MILVUS_PORT` | Milvus server port | `19530` |

### Database Lab Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `DBLAB_ENABLED` | Enable Database Lab integration | `false` |
| `DBLAB_URL` | Database Lab API URL | - |
| `DBLAB_TOKEN` | API token | - |
| `DBLAB_VERIFY_TOKEN` | Verification token | - |
| `DBLAB_TIMEOUT` | Request timeout | `60s` |
| `DBLAB_SEQ_SCAN_ROW_THRESHOLD` | Seq scan warning threshold | `10000` |

### Analysis Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `ANALYSIS_MIN_SEVERITY` | Minimum severity to report | `low` |
| `ANALYSIS_MAX_FILE_SIZE` | Maximum file size (bytes) | `1048576` |
| `ANALYSIS_MIN_CONTENT_LENGTH` | Minimum content length | `50` |
| `ANALYSIS_ENABLE_PERFORMANCE` | Enable performance analysis | `true` |
| `ANALYSIS_ENABLE_SECURITY` | Enable security analysis | `true` |
| `ANALYSIS_ENABLE_STANDARDS` | Enable standards analysis | `true` |
| `ANALYSIS_ORG_STANDARDS_PATH` | Path to org standards file | - |

### Logging Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `LOG_FORMAT` | Log format (json/text) | `json` |
| `LOG_OUTPUT` | Log output (stdout/stderr/file) | `stdout` |
| `LOG_FILE_PATH` | Log file path (if output=file) | - |
| `LOG_ADD_SOURCE` | Add source file to logs | `false` |

### Metrics Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `METRICS_ENABLED` | Enable Prometheus metrics | `true` |
| `METRICS_PORT` | Metrics server port | `9090` |
| `METRICS_PATH` | Metrics endpoint path | `/metrics` |
| `METRICS_NAMESPACE` | Metrics namespace | `code_reviewer` |

### GitLab Integration

| Variable | Description | Default |
|----------|-------------|---------|
| `GITLAB_ENABLED` | Enable GitLab integration | `false` |
| `GITLAB_URL` | GitLab instance URL | - |
| `GITLAB_TOKEN` | GitLab API token | - |
| `GITLAB_TIMEOUT` | API request timeout | `30s` |
| `GITLAB_POST_SUMMARY` | Post review summary comment | `true` |
| `GITLAB_POST_INLINE_COMMENTS` | Post inline code comments | `true` |
| `GITLAB_MIN_SEVERITY` | Minimum severity for comments | `low` |
| `GITLAB_COLLAPSE_THRESHOLD` | Collapse inline if > N issues | `20` |
| `GITLAB_DRY_RUN` | Log comments without posting | `false` |

### Distributed Tracing (OpenTelemetry)

| Variable | Description | Default |
|----------|-------------|---------|
| `TRACING_ENABLED` | Enable distributed tracing | `false` |
| `TRACING_PROVIDER` | Provider (otlp/jaeger/zipkin) | `otlp` |
| `TRACING_ENDPOINT` | Collector endpoint | `localhost:4317` |
| `TRACING_SERVICE_NAME` | Service name in traces | `code-reviewer` |
| `TRACING_SAMPLE_RATE` | Sampling rate (0.0-1.0) | `1.0` |
| `TRACING_INSECURE` | Use insecure connection | `true` |
| `TRACING_BATCH_TIMEOUT` | Batch export timeout | `5s` |

## Health Checks

The service exposes Kubernetes-compatible health check endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /health/live` | Liveness probe - is the service running? |
| `GET /health/ready` | Readiness probe - can it accept traffic? |
| `GET /health/startup` | Startup probe - has initialization completed? |

Example response:
```json
{
  "status": "healthy",
  "checks": {
    "temporal": {"status": "healthy", "latency_ms": 5},
    "vectorstore": {"status": "healthy", "document_count": 1523}
  }
}
```

## Metrics

Prometheus metrics are exposed at `:9090/metrics` (configurable). Key metrics include:

### HTTP Metrics
- `code_reviewer_http_requests_total{method, path, status}` - Total HTTP requests
- `code_reviewer_http_request_duration_seconds{method, path}` - Request latency histogram
- `code_reviewer_http_requests_in_flight` - Current in-flight requests

### Workflow Metrics
- `code_reviewer_workflows_started_total{workflow_type}` - Workflows started
- `code_reviewer_workflows_completed_total{workflow_type, status}` - Workflows completed
- `code_reviewer_workflow_duration_seconds{workflow_type}` - Workflow duration histogram
- `code_reviewer_activities_total{activity, status}` - Activities executed

### LLM Metrics
- `code_reviewer_llm_requests_total{model, status}` - LLM API calls
- `code_reviewer_llm_request_duration_seconds{model}` - LLM latency histogram
- `code_reviewer_llm_tokens_total{model, type}` - Token usage
- `code_reviewer_llm_errors_total{model, error_type}` - LLM errors

### Embedding Metrics
- `code_reviewer_embedding_requests_total{model, status}` - Embedding requests
- `code_reviewer_embedding_cache_hits_total` - Cache hits
- `code_reviewer_embedding_cache_misses_total` - Cache misses
- `code_reviewer_embedding_cache_size` - Current cache size

### Analysis Metrics
- `code_reviewer_analysis_issues_found_total{category, severity}` - Issues found
- `code_reviewer_analysis_duration_seconds{category}` - Analysis duration

## GitLab Integration

### Setup

1. Create a GitLab API token with `api` scope
2. Configure the integration:
   ```bash
   export GITLAB_ENABLED=true
   export GITLAB_URL=https://gitlab.yourcompany.com
   export GITLAB_TOKEN=glpat-xxxxx
   ```
3. Configure GitLab webhook to POST to your service's `/webhook` endpoint

### Review Comment Format

Summary comments include:
- Issue count by category (Performance, Security, Standards)
- Severity breakdown with icons (Critical, High, Medium, Low)
- Collapsible details for each issue

Inline comments include:
- Severity indicator
- Problem description
- Impact analysis
- Suggested fix
- Reference links (CWE for security issues)

### Dry Run Mode

Test the integration without posting comments:
```bash
export GITLAB_DRY_RUN=true
```
Comments will be logged but not posted to GitLab.

## Distributed Tracing

### Jaeger Setup

```bash
# Start Jaeger
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest

# Configure tracing
export TRACING_ENABLED=true
export TRACING_ENDPOINT=localhost:4317
export TRACING_SERVICE_NAME=code-reviewer
export TRACING_SAMPLE_RATE=1.0
```

Access the Jaeger UI at `http://localhost:16686`.

### Trace Propagation

Traces propagate through:
- HTTP requests (via W3C Trace Context headers)
- Temporal workflows and activities
- LLM and embedding requests
- Vector store operations

## Graceful Shutdown

The service implements graceful shutdown with:

1. **Request Draining**: Stops accepting new requests, waits for in-flight requests
2. **Ordered Hook Execution**: Shutdown hooks execute in priority order
3. **Timeout Protection**: Configurable timeout prevents hanging

Shutdown order:
1. Stop accepting new HTTP requests (priority 10)
2. Wait for in-flight requests to complete
3. Stop Temporal workers (priority 20)
4. Flush metrics and traces (priority 50)
5. Close database connections (priority 100)

## Project Structure

```
.
├── cmd/
│   └── postgres-worker/     # Temporal worker binary
├── internal/
│   ├── atomicfile/          # Atomic file operations
│   ├── config/              # Configuration management
│   ├── errors/              # Error handling, retry, circuit breaker
│   ├── gitlab/              # GitLab API client and reviewer
│   ├── health/              # Health check endpoints
│   ├── logging/             # Structured logging (slog)
│   ├── metrics/             # Prometheus metrics
│   ├── postgres/            # PostgreSQL analyzers
│   ├── rag/                 # RAG system (chunker, embedder, retriever)
│   ├── shutdown/            # Graceful shutdown manager
│   ├── tracing/             # OpenTelemetry integration
│   └── workflow/            # Temporal workflows/activities
├── data/                    # Persistent data (vector store, cache)
└── docs/                    # Additional documentation
```

## Dependencies

### Core Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `go.temporal.io/sdk` | v1.38.0 | Workflow orchestration |
| `github.com/prometheus/client_golang` | v1.23.2 | Metrics instrumentation |
| `go.opentelemetry.io/otel` | v1.39.0 | Distributed tracing |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | v1.39.0 | OTLP trace export |
| `github.com/milvus-io/milvus-sdk-go/v2` | v2.4.1 | Vector database (optional) |
| `google.golang.org/grpc` | v1.77.0 | gRPC client |

### Standard Library Features Used

- `log/slog` - Structured logging (Go 1.21+)
- `sync/atomic` - Lock-free counters
- `context` - Cancellation and timeouts
- `net/http` - HTTP server and client
- `encoding/json` - JSON serialization
- `text/template` - Prompt templates

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/rag/...
go test ./internal/postgres/...
go test ./internal/gitlab/...
```

### Building

```bash
# Build all binaries
go build -o bin/ ./cmd/...

# Build with version info
go build -ldflags "-X main.Version=1.0.0" -o bin/ ./cmd/...
```

## Troubleshooting

### LLM Connection Issues

```bash
# Check Ollama is running
curl http://localhost:11434/api/tags

# Test model availability
ollama run qwen2.5-coder:32b "SELECT 1"
```

### Temporal Connection Issues

```bash
# Check Temporal is running
temporal operator namespace list

# Check worker registration
temporal task-queue describe --task-queue code-review-queue
```

### Tracing Not Appearing

```bash
# Verify collector endpoint
grpcurl -plaintext localhost:4317 list

# Check service logs for export errors
export LOG_LEVEL=debug
```

### GitLab API Errors

```bash
# Test API token
curl -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "https://gitlab.example.com/api/v4/user"

# Enable dry run to test without posting
export GITLAB_DRY_RUN=true
```

## License

MIT
