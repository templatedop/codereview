# On-Premise Code Review System

An AI-powered code review system using DeepSeek LLM with Temporal workflow orchestration and agentic capabilities.

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
```

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
# Terminal 1: Start the worker
./bin/worker -ollama http://localhost:11434 -model deepseek-coder:1.3b

# Terminal 2: Trigger a review
./bin/trigger -file path/to/code.go

# With agentic multi-step reasoning
./bin/trigger -file code.go -agent

# With framework context
./bin/trigger -file code.go -framework gin -agent
```

### 4. GitLab Integration

For automated MR reviews via webhooks:

```bash
# Start the webhook server
./bin/server -port 8080 -gitlab-token YOUR_TOKEN

# Configure GitLab webhook to POST to:
# http://your-server:8080/webhook
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OLLAMA_URL` | Ollama API endpoint | `http://localhost:11434` |
| `LLM_MODEL` | Model to use | `deepseek-coder:1.3b` |
| `TEMPORAL_HOST` | Temporal server address | `localhost:7233` |
| `GITLAB_URL` | GitLab instance URL | `https://gitlab.com` |
| `GITLAB_TOKEN` | GitLab API token | - |

### CLI Flags

**reviewer:**
```
-file string      Path to file to review
-model string     LLM model name (default "deepseek-coder:1.3b")
-ollama string    Ollama URL (default "http://localhost:11434")
-framework string Framework name for context-aware review
```

**worker:**
```
-ollama string    Ollama URL (default "http://localhost:11434")
-model string     LLM model (default "deepseek-coder:1.3b")
-temporal string  Temporal host (default "localhost:7233")
-queue string     Task queue name (default "code-review")
```

**trigger:**
```
-file string      Path to file to review
-framework string Framework for context
-agent            Enable agentic multi-step review
-temporal string  Temporal host (default "localhost:7233")
```

**indexer:**
```
index -name string -repo string    Index a GitHub repository
list                               List indexed frameworks
search -name string -query string  Search indexed code
```

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

## Output Format

Reviews are returned as JSON:

```json
{
  "issues": [
    {
      "type": "security",
      "severity": "high",
      "line": 42,
      "message": "SQL injection vulnerability: user input directly concatenated",
      "suggestion": "Use parameterized queries instead"
    }
  ],
  "summary": "Found 1 critical security issue",
  "risk_score": 8
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
│   └── gitlab/        # GitLab API client
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

## License

MIT
