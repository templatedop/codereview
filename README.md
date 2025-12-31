# On-Premise Code Review System

An AI-powered code review system using DeepSeek LLM with Temporal workflow orchestration, GitLab integration, and agentic capabilities.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   GitLab/CLI    │────▶│    Temporal     │────▶│     Worker      │
│   (Trigger)     │     │    Server       │     │   (Activities)  │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                                                         │
                        ┌─────────────────┐              │
                        │  Knowledge Store │◀─────────────┤
                        │  (Framework RAG) │              │
                        └─────────────────┘              │
                                                         ▼
                                                ┌─────────────────┐
                                                │   DeepSeek LLM  │
                                                │   (via Ollama)  │
                                                └─────────────────┘
                                                         │
                                                         ▼
                                                ┌─────────────────┐
                                                │     GitLab      │
                                                │  (Post Reviews) │
                                                └─────────────────┘
```

## Features

- **AI-Powered Reviews**: Uses DeepSeek LLM for intelligent code analysis
- **GitLab Integration**: Automatically post reviews to merge requests
- **Framework-Aware**: Index your codebase for context-aware reviews
- **Agentic Mode**: Multi-step reasoning with specialized tools
- **Configurable Logging**: Multiple log levels, formats, and outputs
- **Graceful Shutdown**: Clean shutdown handling for all services

## Components

| Component | Description |
|-----------|-------------|
| `cmd/reviewer` | CLI for direct code review without Temporal |
| `cmd/worker` | Temporal worker that processes review workflows |
| `cmd/trigger` | CLI to start Temporal workflows |
| `cmd/indexer` | Index GitHub repositories for framework-aware reviews |
| `cmd/server` | HTTP server for GitLab webhook integration |

## Prerequisites

- **Go 1.21+**
- **Ollama** with DeepSeek model installed
- **Temporal** (optional, for workflow orchestration)
- **GitLab** (optional, for MR integration)

### Install Ollama and DeepSeek

```bash
# Install Ollama (https://ollama.ai)
curl -fsSL https://ollama.ai/install.sh | sh

# Pull DeepSeek model
ollama pull deepseek-coder:1.3b
# or larger model
ollama pull deepseek-coder:6.7b
```

### Install Temporal (Optional)

```bash
# Using Docker
docker run -d --name temporal \
  -p 7233:7233 \
  temporalio/auto-setup:latest
```

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd code-reviewer

# Build all binaries
go build -o bin/ ./cmd/...

# Run tests
go test ./...
```

## Usage

### 1. Direct Code Review (No Temporal)

The simplest way to review code:

```bash
# Review a single file
./bin/reviewer -file path/to/code.go

# Specify model
./bin/reviewer -file code.go -model deepseek-coder:6.7b

# With framework context (after indexing)
./bin/reviewer -file code.go -framework gin

# With debug logging
./bin/reviewer -file code.go -log-level debug
```

### 2. Index a Framework (RAG)

Index a GitHub repository to provide framework-aware reviews:

```bash
# Index the Gin framework
./bin/indexer index -name gin -repo gin-gonic/gin

# Index your internal framework
./bin/indexer index -name myframework -repo yourorg/framework

# List indexed frameworks
./bin/indexer list

# Search indexed code
./bin/indexer search -name gin -query "middleware"
```

### 3. Temporal Workflow (Production)

For production use with workflow orchestration:

```bash
# Terminal 1: Start the worker with GitLab integration
./bin/worker \
  -llm-url http://localhost:11434 \
  -model deepseek-coder:1.3b \
  -gitlab-url https://gitlab.yourcompany.com \
  -gitlab-token YOUR_TOKEN \
  -log-level info \
  -log-format json

# Terminal 2: Trigger a review
./bin/trigger -file path/to/code.go

# With agentic multi-step reasoning
./bin/trigger -file code.go -agent

# With framework context
./bin/trigger -file code.go -framework gin -agent
```

### 4. GitLab Integration

#### Via Webhooks (Automatic MR Reviews)

```bash
# Start the webhook server
./bin/server \
  -addr :8080 \
  -gitlab-url https://gitlab.yourcompany.com \
  -gitlab-token YOUR_TOKEN \
  -log-level info

# Configure GitLab webhook:
# 1. Go to Project > Settings > Webhooks
# 2. URL: http://your-server:8080/webhook
# 3. Trigger: Merge request events
```

#### Via API (Manual Reviews with GitLab Posting)

```bash
# Review and post to GitLab MR
curl -X POST http://localhost:8080/api/review \
  -H "Content-Type: application/json" \
  -d '{
    "file_path": "main.go",
    "diff": "+func insecure() { exec.Command(userInput) }",
    "project_id": 123,
    "merge_request_id": 42,
    "post_to_mr": true
  }'

# Batch review multiple files
curl -X POST http://localhost:8080/api/review/batch \
  -H "Content-Type: application/json" \
  -d '{
    "files": [
      {"file_path": "a.go", "diff": "+code"},
      {"file_path": "b.go", "diff": "+more code"}
    ],
    "project_id": 123,
    "merge_request_id": 42,
    "post_to_mr": true
  }'
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_URL` | Ollama API endpoint | `http://localhost:11434` |
| `LLM_MODEL` | Model to use | `deepseek-coder:1.3b` |
| `TEMPORAL_HOST` | Temporal server address | `localhost:7233` |
| `GITLAB_URL` | GitLab instance URL | - |
| `GITLAB_TOKEN` | GitLab API token | - |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `LOG_FORMAT` | Log format (text, json) | `text` |
| `LOG_OUTPUT` | Log output (stdout, stderr, file path) | `stdout` |
| `KNOWLEDGE_DIR` | Directory for framework data | `~/.code-reviewer/knowledge` |

### CLI Flags

**reviewer:**
```
-file string        Path to file to review
-model string       LLM model name (default "deepseek-coder:1.3b")
-ollama string      Ollama URL (default "http://localhost:11434")
-framework string   Framework name for context-aware review
-log-level string   Log level: debug, info, warn, error (default "info")
```

**worker:**
```
-llm-url string     LLM URL (default "http://localhost:11434")
-model string       LLM model (default "deepseek-coder:6.7b")
-temporal string    Temporal host (default "localhost:7233")
-gitlab-url string  GitLab server URL
-gitlab-token string GitLab access token
-knowledge-dir string Knowledge store directory
-log-level string   Log level (default "info")
-log-format string  Log format: text, json (default "text")
-log-output string  Log output: stdout, stderr, or file path
```

**server:**
```
-addr string        Server address (default ":8080")
-llm-url string     LLM URL (default "http://localhost:11434")
-model string       LLM model (default "deepseek-coder:1.3b")
-gitlab-url string  GitLab server URL
-gitlab-token string GitLab access token
-log-level string   Log level (default "info")
-log-format string  Log format: text, json (default "text")
-log-output string  Log output: stdout, stderr, or file path
```

**trigger:**
```
-file string        Path to file to review
-framework string   Framework for context
-agent              Enable agentic multi-step review
-temporal string    Temporal host (default "localhost:7233")
```

**indexer:**
```
index -name string -repo string    Index a GitHub repository
list                               List indexed frameworks
search -name string -query string  Search indexed code
```

## Logging

The system supports configurable logging with multiple levels and output formats.

### Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Verbose debugging information |
| `info` | Normal operational messages |
| `warn` | Warning messages |
| `error` | Error messages |

### Log Formats

**Text format (default):**
```
2024-01-15 10:30:45 [INFO] Starting Code Review Worker...
2024-01-15 10:30:45 [INFO] LLM: http://localhost:11434 (model: deepseek-coder:1.3b)
```

**JSON format:**
```json
{"time":"2024-01-15T10:30:45Z","level":"INFO","msg":"Starting Code Review Worker..."}
{"time":"2024-01-15T10:30:45Z","level":"INFO","msg":"LLM: http://localhost:11434 (model: deepseek-coder:1.3b)"}
```

### Log to File

```bash
./bin/worker -log-output /var/log/code-reviewer/worker.log
```

## Graceful Shutdown

All services support graceful shutdown:

- **Worker**: Completes in-progress tasks before stopping (30s timeout)
- **Server**: Finishes active HTTP requests before stopping (30s timeout)

Send `SIGINT` (Ctrl+C) or `SIGTERM` to trigger graceful shutdown.

## Agent Tools

When using `-agent` mode, the system uses a multi-step reasoning approach with these tools:

| Tool | Description |
|------|-------------|
| `search_framework` | Search indexed framework code for patterns and best practices |
| `analyze_security` | Deep security vulnerability analysis |
| `analyze_performance` | Performance issue detection |
| `suggest_fix` | Generate code fixes for identified issues |
| `final_review` | Compile findings into final review result |

The agent performs up to 5 reasoning steps, selecting appropriate tools based on the code being reviewed.

## GitLab Review Format

When posting reviews to GitLab, the system creates:

1. **Summary Comment**: Overview with issue counts by severity
2. **Inline Comments**: Comments on specific lines (when possible)

Example summary posted to MR:

```markdown
## 🤖 Automated Code Review

**Files Reviewed:** 3 / 3
**Issues Found:** 5

### Issues by Severity

| Severity | Count |
|----------|-------|
| 🔴 Critical | 1 |
| 🟠 High | 2 |
| 🟡 Medium | 1 |
| 🟢 Low | 1 |

### Detailed Findings

#### 📄 `db/query.go`

- 🔴 **[SECURITY]** SQL Injection (line 42)
  - User input directly concatenated into SQL query
  - 💡 Use parameterized queries instead
```

## Output Format

Reviews are returned as JSON:

```json
{
  "issues": [
    {
      "type": "security",
      "severity": "high",
      "line": 42,
      "title": "SQL Injection",
      "description": "User input directly concatenated into SQL query",
      "suggestion": "Use parameterized queries instead"
    }
  ],
  "summary": "Found 1 critical security issue",
  "risk_score": 8,
  "recommendation": "Fix security issues before merging"
}
```

## Project Structure

```
.
├── cmd/
│   ├── reviewer/      # Direct review CLI
│   ├── worker/        # Temporal worker
│   ├── trigger/       # Workflow trigger CLI
│   ├── indexer/       # Framework indexer CLI
│   └── server/        # GitLab webhook server
├── internal/
│   ├── llm/           # LLM client (Ollama/OpenAI)
│   ├── reviewer/      # Core review logic
│   ├── agent/         # Agentic reasoning with tools
│   ├── workflow/      # Temporal workflows & activities
│   ├── knowledge/     # Framework knowledge store
│   ├── indexer/       # Go AST parser for indexing
│   ├── github/        # GitHub repo cloning
│   ├── gitlab/        # GitLab API client & review poster
│   └── logger/        # Configurable logging
├── samples/           # Sample vulnerable code for testing
└── frameworks/        # Indexed framework data (generated)
```

## Sample Vulnerable Files

Test the system with provided samples:

```bash
./bin/reviewer -file samples/sql_injection.go
./bin/reviewer -file samples/command_injection.go
./bin/reviewer -file samples/hardcoded_secrets.go
./bin/reviewer -file samples/xss_vulnerability.go
./bin/reviewer -file samples/concurrency_issues.go
```

## Troubleshooting

### LLM returns malformed JSON

The system handles common LLM quirks:
- JSON with `//` comments (stripped automatically)
- Numbers as strings (`"10"` vs `10`)
- Extra text around JSON (extracted automatically)

### Ollama connection refused

Ensure Ollama is running:
```bash
ollama serve
# or check if already running
curl http://localhost:11434/api/tags
```

### Temporal connection failed

Ensure Temporal is running:
```bash
docker ps | grep temporal
# or start it
docker run -d -p 7233:7233 temporalio/auto-setup:latest
```

### GitLab authentication failed

Verify your token has the required permissions:
- `api` scope for full API access
- Or at minimum: `read_api`, `read_repository`, `write_repository`

### Reviews not posting to GitLab

Check:
1. GitLab URL and token are configured
2. Token has permission to comment on the project
3. Project ID and MR IID are correct
4. Check logs for specific errors (`-log-level debug`)

## License

MIT
