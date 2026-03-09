# API Keys Setup Guide

RecurseAI requires API access to LLM providers for embeddings and text generation. Here's how to set up your API keys.

## ⚠️ Important: Embeddings Requirement

**RecurseAI requires a provider that supports embeddings for document ingestion.** Currently supported:

- ✅ **OpenAI** - Recommended (uses `text-embedding-3-small`)
- ✅ **Ollama** - Free local option (requires running Ollama server)
- ❌ **Anthropic** - No embeddings API available
- ⚠️ **Gemini** - Embeddings API currently has compatibility issues

## Quick Setup (Recommended)

### Option 1: OpenAI (Easiest)

```bash
# Set environment variable
export OPENAI_API_KEY="sk-..."

# Update config to use OpenAI for embeddings
# Edit recurseai.yaml:
tree:
  summarizeProvider: openai  # This handles both embeddings AND summarization
  summarizeModel: gpt-4o-mini
```

**Get API Key:** https://platform.openai.com/api-keys

**Cost:** ~$0.0001 per 1K tokens (embeddings), ~$0.15 per 1M tokens (gpt-4o-mini)

### Option 2: Local Ollama (Free)

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Pull a model
ollama pull llama3

# Start Ollama (runs on localhost:11434 by default)
ollama serve

# Update config
tree:
  summarizeProvider: ollama
  summarizeModel: llama3

query:
  answerProvider: ollama
  answerModel: llama3
```

**No API key needed!** Everything runs locally.

## Environment Variables

You can set API keys via environment variables instead of the YAML file:

```bash
# Mac/Linux
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AI..."

# Windows
set OPENAI_API_KEY=sk-...
set ANTHROPIC_API_KEY=sk-ant-...
set GEMINI_API_KEY=AI...
```

## Hybrid Configuration (Recommended)

Use OpenAI for embeddings (reliable) and cheaper providers for generation:

```yaml
tree:
  summarizeProvider: openai  # Embeddings + summarization
  summarizeModel: gpt-4o-mini

query:
  answerProvider: anthropic  # High-quality final answers
  answerModel: claude-3-5-sonnet-20241022

providers:
  openai:
    apiKey: "sk-..."  # Or via OPENAI_API_KEY env var
    model: gpt-4o-mini
  anthropic:
    apiKey: "sk-ant-..."
    model: claude-3-5-sonnet-20241022
```

## Getting API Keys

### OpenAI
1. Go to https://platform.openai.com/api-keys
2. Click "Create new secret key"
3. Copy the key (starts with `sk-proj-...`)
4. **Add billing:** https://platform.openai.com/account/billing

### Anthropic (Claude)
1. Go to https://console.anthropic.com/
2. Click "Get API keys"
3. Copy the key (starts with `sk-ant-...`)

### Google Gemini
1. Go to https://aistudio.google.com/app/apikey
2. Create an API key
3. Copy the key (starts with `AI...`)

## Testing Your Setup

```bash
# Test with environment variables
export OPENAI_API_KEY="sk-..."

# Run the test
./test_run.sh

# Or test manually
./recurseai ingest --file ./testdata/sample.txt
./recurseai query --question "What is this document about?"
```

## Common Issues

### "exceeded your current quota"
**Problem:** OpenAI API key has no credits or billing not set up.

**Solution:** 
1. Go to https://platform.openai.com/account/billing
2. Add payment method and credits
3. Or use Ollama (free local option)

### "provider not found"
**Problem:** API key not set or config malformed.

**Solution:**
1. Check YAML uses `apiKey` (camelCase), not `api_key`
2. Verify environment variables are exported
3. Run `./recurseai --help` to check if binary is using latest config

### "Anthropic embed not supported"
**Problem:** Trying to use Anthropic for embeddings.

**Solution:** Anthropic doesn't provide embeddings. Use OpenAI or Ollama for the `summarizeProvider` in tree config.

### "Gemini embed error: model not found"
**Problem:** Gemini embeddings API has compatibility issues.

**Solution:** Use OpenAI or Ollama for embeddings instead.

## Cost Comparison

| Provider | Embeddings | Generation | Use Case |
|----------|-----------|-----------|----------|
| **OpenAI** | $0.0001/1K tokens | $0.15/1M tokens (mini) | Best reliability |
| **Anthropic** | ❌ Not available | $3/1M tokens | Highest quality answers |
| **Gemini** | ⚠️ Issues | $0.075/1M tokens | Cheapest (when working) |
| **Ollama** | ✅ Free | ✅ Free | Local, privacy-focused |

## Recommended Production Setup

```yaml
# Use OpenAI for embeddings (reliable)
tree:
  summarizeProvider: openai
  summarizeModel: gpt-4o-mini

# Use Anthropic for high-quality answers
query:
  answerProvider: anthropic
  answerModel: claude-3-5-sonnet-20241022

providers:
  openai:
    apiKey: ""  # Set via OPENAI_API_KEY env var
    model: gpt-4o-mini
  anthropic:
    apiKey: ""  # Set via ANTHROPIC_API_KEY env var
    model: claude-3-5-sonnet-20241022
```

**Cost for 100 document ingestions + 1000 queries:**
- Embeddings (OpenAI): ~$0.50
- Summarization (OpenAI gpt-4o-mini): ~$1.50
- Answers (Anthropic Claude): ~$10
- **Total: ~$12**

## Security Best Practices

1. **Never commit API keys to git**
2. Use environment variables for production
3. Add `recurseai.yaml` to `.gitignore` if it contains keys
4. Rotate keys regularly
5. Set spending limits in provider dashboards

---

**Need help?** Check [SETUP_GUIDE.md](SETUP_GUIDE.md) for full installation instructions.
