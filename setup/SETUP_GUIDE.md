# RecurseAI — Complete Setup & Run Guide

Recursive context compression engine for long documents with hierarchical summarization.

---

## Quick Start

### Prerequisites
- **Go 1.24+** — Download from https://go.dev/dl/
- **GCC/Clang** (for SQLite CGO) — Usually pre-installed on Mac/Linux
  - Mac: `xcode-select --install`
  - Ubuntu: `sudo apt install build-essential`
  - Windows: Install TDM-GCC or MinGW
- **API Keys** — At least one of:
  - OpenAI: https://platform.openai.com/api-keys
  - Anthropic: https://console.anthropic.com/
  - Google Gemini: https://aistudio.google.com/app/apikey
  - Or local Ollama: https://ollama.ai/

---

## Installation

### 1. Clone & Install Dependencies
```bash
cd recurseai
go mod tidy
```

All dependencies are already tracked in `go.mod`. Running `go mod tidy` ensures everything is downloaded.

### 2. Set Up Environment Variables
Export your API keys (choose the providers you want to use):

**Mac/Linux** — Add to `~/.zshrc` or `~/.bashrc`:
```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AI..."

# Then reload
source ~/.zshrc
```

**Windows PowerShell**:
```powershell
$env:OPENAI_API_KEY="sk-..."
$env:ANTHROPIC_API_KEY="sk-ant-..."
$env:GEMINI_API_KEY="AI..."
```

> **Note:** API keys in `recurseai.yaml` are only used as fallbacks. Environment variables take precedence.

### 3. Configure Settings
Edit `recurseai.yaml` to customize:
- Chunk sizes and overlap
- Tree branching factor and depth
- Which provider to use for summarization vs. answering
- Beam search parameters

### 4. Build the Binary
```bash
go build -o recurseai ./cmd
```

Verify it works:
```bash
./recurseai --help
```

---

## Usage

### Ingest a Document
```bash
./recurseai ingest --file ./testdata/sample.txt --config recurseai.yaml
```

**Output:**
```
Ingested doc=sample.txt-a3f9b2 chunks=42 nodes=56 took=8.3s
```

### Query the Document
```bash
./recurseai query \
  --question "What are the main topics covered?" \
  --config recurseai.yaml
```

**Output:**
```
Answer:
The document covers recursive summarization techniques, hierarchical tree structures, 
parallel processing with Go goroutines, and multi-provider LLM support...

Provider: anthropic
Tokens: 245
Sources:
- chunk-a1b2c3 (testdata/sample.txt)
- chunk-d4e5f6 (testdata/sample.txt)
```

### Override Provider
```bash
# Use OpenAI instead of default
./recurseai query \
  --question "Summarize the key features" \
  --provider openai

# Use local Ollama (no API cost)
./recurseai query \
  --question "Summarize the key features" \
  --provider ollama
```

### Start REST API Server
```bash
./recurseai serve --port 8080
```

**Test with curl:**
```bash
# Ingest
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{"file_path": "./testdata/sample.txt"}'

# Query
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"question": "What is this about?"}'

# Health check
curl http://localhost:8080/health

# List documents
curl http://localhost:8080/docs
```

---

## Testing

### Run Automated Smoke Test
```bash
./test_run.sh
```

This will:
1. Build the binary
2. Ingest a test document
3. Query it
4. Verify everything works end-to-end

### Run Go Tests
```bash
# All tests
go test ./...

# Verbose
go test ./... -v

# With race detector
go test ./... -race

# Specific package
go test ./core/... -v
```

---

## Project Structure

```
recurseai/
├── go.mod                      # Dependencies
├── go.sum                      # Checksums
├── recurseai.yaml              # Configuration
├── test_run.sh                 # Smoke test script
├── data/                       # SQLite databases (created on first run)
│   ├── vectors.db
│   └── tree.db
├── testdata/                   # Test documents
│   └── sample.txt
├── cmd/                        # CLI entrypoint
│   ├── main.go
│   ├── ingest.go
│   └── query.go
├── core/                       # Core engine logic
│   ├── engine.go               # Main orchestrator
│   ├── chunker.go              # Text splitting
│   ├── tree.go                 # Tree builder
│   ├── traversal.go            # Beam search
│   └── context_builder.go      # Context assembly
├── providers/                  # LLM provider implementations
│   ├── interface.go
│   ├── registry.go
│   ├── openai/provider.go
│   ├── anthropic/provider.go
│   ├── gemini/provider.go
│   └── ollama/provider.go
├── workers/                    # Concurrency primitives
│   ├── pool.go                 # Worker pool
│   └── semaphore.go            # Semaphore
├── store/                      # Persistence layer
│   ├── vector.go               # Vector DB
│   └── tree.go                 # Tree storage
├── config/                     # Configuration
│   └── config.go
└── api/                        # REST API
    └── server.go
```

---

## Common Issues & Fixes

### `cgo: C compiler not found`
**Solution:** Install GCC/Clang:
- Mac: `xcode-select --install`
- Ubuntu: `sudo apt install build-essential`
- Windows: Install TDM-GCC

### `undefined provider in registry`
**Solution:** Check `recurseai.yaml` — provider names must be exact:
- `openai`, `anthropic`, `gemini`, `ollama` (lowercase)

### `no such table: vectors`
**Solution:** Run `ingest` before `query`. The database is created on first ingest.

### `Anthropic embed not supported`
**Expected behavior.** Anthropic doesn't provide embeddings. The system handles this gracefully — embeddings will be skipped or use a fallback provider if configured.

### `context deadline exceeded` with Ollama
**Solution:** Ollama can be slow. Increase timeout in `providers/ollama/provider.go`:
```go
httpClient: &http.Client{
    Timeout: 300 * time.Second, // Increase from 120s
}
```

### `module not found` errors
**Solution:**
```bash
go mod tidy
go mod download
```

---

## Advanced Configuration

### `recurseai.yaml` Explained

```yaml
ingestion:
  chunkSize: 768           # Tokens per chunk
  chunkOverlap: 64         # Overlap between chunks
  workerPoolSize: 10       # Parallel embedding workers

tree:
  branchFactor: 4          # Children per parent node
  maxLevels: 5             # Max tree depth
  summarizeProvider: gemini # Which LLM for summaries
  summarizeModel: gemini-2.0-flash

query:
  beamWidth: 3             # Top-K branches to explore
  similarityThreshold: 0.72 # Minimum cosine similarity
  answerProvider: anthropic # Which LLM for final answer
  answerModel: claude-3-5-sonnet-20241022

providers:
  openai:
    apiKey: ""             # Uses OPENAI_API_KEY env var
    model: gpt-4o
  anthropic:
    apiKey: ""             # Uses ANTHROPIC_API_KEY env var
    model: claude-3-5-sonnet-20241022
  gemini:
    apiKey: ""             # Uses GEMINI_API_KEY env var
    model: gemini-2.0-flash
  ollama:
    baseURL: http://localhost:11434
    model: llama3.1

storage:
  vectorDB: ./data/vectors.db
  treeDB: ./data/tree.db
```

### Performance Tuning

**For faster ingestion:**
- Increase `workerPoolSize` (default: 10)
- Use faster embedding models (e.g., `text-embedding-3-small`)
- Increase `chunkSize` for fewer chunks

**For better retrieval:**
- Increase `beamWidth` (explores more branches)
- Lower `similarityThreshold` (includes more nodes)
- Increase `tree.maxLevels` (deeper hierarchy)

**For lower costs:**
- Use Gemini for summaries (cheaper than GPT-4)
- Use Ollama for local inference (free)
- Reduce `workerPoolSize` (slower but fewer parallel API calls)

---

## Next Steps

1. **Try with your own documents:**
   ```bash
   ./recurseai ingest --file /path/to/your/document.txt
   ./recurseai query --question "Your question here"
   ```

2. **Integrate into your application:**
   - Use the REST API (`./recurseai serve`)
   - Or import as a Go library

3. **Customize for your use case:**
   - Adjust chunk sizes for your document types
   - Experiment with different providers
   - Tune beam search parameters

---

## License

MIT

## Contributing

Issues and pull requests welcome!
