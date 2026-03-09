#!/bin/bash
set -e

echo "=== RecurseAI Smoke Test ==="

# 1. Create a test document
echo "Creating test document..."
cat > /tmp/test_doc.txt << 'EOF'
The quick brown fox jumped over the lazy dog.
This is a test document with multiple paragraphs.

RecurseAI uses recursive summarization to handle long documents.
Go's goroutines make this process fast and efficient.

The system builds hierarchical trees for efficient retrieval.
Each level provides progressively more abstract summaries.

This approach handles documents of any length effectively.
Traditional RAG systems struggle with very long contexts.

Key benefits include:
- Fast parallel processing
- Efficient memory usage
- Multi-provider support
- SQLite-based persistence
EOF

echo "✓ Test document created"

# 2. Build
echo ""
echo "Building..."
go build -o ./recurseai ./cmd
echo "✓ Build successful"

# 3. Verify binary exists
if [ ! -f ./recurseai ]; then
    echo "✗ Binary not found"
    exit 1
fi
echo "✓ Binary verified"

# 4. Test help command
echo ""
echo "Testing help command..."
./recurseai --help > /dev/null
echo "✓ Help command works"

# 5. Ingest
echo ""
echo "Ingesting test document..."
echo "(This may take a minute depending on your API provider...)"
./recurseai ingest --file /tmp/test_doc.txt --provider openai --config recurseai.yaml
echo "✓ Ingest successful"

# 6. Query
echo ""
echo "Querying ingested document..."
./recurseai query --question "What is this document about?" --config recurseai.yaml
echo ""
echo "✓ Query successful"

echo ""
echo "=== All tests passed ==="
echo ""
echo "Next steps:"
echo "  1. Try: ./recurseai ingest --file ./testdata/sample.txt"
echo "  2. Try: ./recurseai query --question 'What are the key features?'"
echo "  3. Try: ./recurseai serve --port 8080"
