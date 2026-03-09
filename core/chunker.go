package core

import (
	"crypto/md5"
	"fmt"
	"os"
	"strings"
)

// Chunk represents a piece of text extracted from a document
type Chunk struct {
	ID         string
	Text       string
	TokenCount int
	Index      int // position in original doc
	SourceFile string
}

// ChunkerConfig defines configuration for text chunking
type ChunkerConfig struct {
	ChunkSize int    // tokens per chunk
	Overlap   int    // overlap between chunks
	Separator string // split on "\n\n" by default
}

// Chunker handles text chunking operations
type Chunker struct {
	config ChunkerConfig
}

// NewChunker creates a new Chunker instance with the given configuration
func NewChunker(config ChunkerConfig) *Chunker {
	if config.Separator == "" {
		config.Separator = "\n\n"
	}
	if config.ChunkSize == 0 {
		config.ChunkSize = 512
	}
	return &Chunker{
		config: config,
	}
}

// Chunk splits text into chunks based on configuration
func (c *Chunker) Chunk(text string) []Chunk {
	// Split text by separator
	parts := c.splitBySeparator(text)

	// Merge small chunks
	parts = c.mergeSmallChunks(parts)

	chunks := make([]Chunk, 0)
	currentPos := 0

	for i, part := range parts {
		tokenCount := c.estimateTokens(part)

		// If part is larger than chunk size, split it further
		if tokenCount > c.config.ChunkSize {
			subChunks := c.splitLargeChunk(part, i)
			chunks = append(chunks, subChunks...)
			currentPos += len(subChunks)
		} else {
			// Create chunk with overlap handling
			chunk := Chunk{
				ID:         c.generateChunkID(part, i),
				Text:       part,
				TokenCount: tokenCount,
				Index:      currentPos,
				SourceFile: "",
			}
			chunks = append(chunks, chunk)
			currentPos++
		}
	}

	return chunks
}

// ChunkFile reads a file and chunks its content
func (c *Chunker) ChunkFile(path string) ([]Chunk, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	chunks := c.Chunk(string(content))

	// Set source file for all chunks
	for i := range chunks {
		chunks[i].SourceFile = path
	}

	return chunks, nil
}

// estimateTokens provides a rough estimate of token count
// Uses approximately 4 characters per token as a heuristic
func (c *Chunker) estimateTokens(text string) int {
	// Rough approximation: ~4 chars per token for English text
	charCount := len(text)
	return (charCount + 3) / 4
}

// splitBySeparator splits text by the configured separator
func (c *Chunker) splitBySeparator(text string) []string {
	parts := strings.Split(text, c.config.Separator)

	// Remove empty parts
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// mergeSmallChunks combines adjacent small chunks to optimize chunk size
func (c *Chunker) mergeSmallChunks(parts []string) []string {
	if len(parts) == 0 {
		return parts
	}

	minSize := c.config.ChunkSize / 4 // Merge if less than 25% of target
	result := make([]string, 0, len(parts))
	current := ""

	for _, part := range parts {
		if current == "" {
			current = part
			continue
		}

		currentTokens := c.estimateTokens(current)
		partTokens := c.estimateTokens(part)

		// If current is small and combining won't exceed chunk size, merge
		if currentTokens < minSize && (currentTokens+partTokens) <= c.config.ChunkSize {
			current = current + c.config.Separator + part
		} else {
			result = append(result, current)
			current = part
		}
	}

	// Add the last chunk
	if current != "" {
		result = append(result, current)
	}

	return result
}

// splitLargeChunk splits a large chunk into smaller ones
func (c *Chunker) splitLargeChunk(text string, baseIndex int) []Chunk {
	chunks := make([]Chunk, 0)
	words := strings.Fields(text)

	current := ""
	chunkIndex := 0

	for _, word := range words {
		testText := current
		if testText != "" {
			testText += " "
		}
		testText += word

		if c.estimateTokens(testText) > c.config.ChunkSize && current != "" {
			// Save current chunk
			chunk := Chunk{
				ID:         c.generateChunkID(current, baseIndex+chunkIndex),
				Text:       current,
				TokenCount: c.estimateTokens(current),
				Index:      baseIndex + chunkIndex,
				SourceFile: "",
			}
			chunks = append(chunks, chunk)
			chunkIndex++

			// Start new chunk with overlap if configured
			if c.config.Overlap > 0 {
				overlapWords := c.getLastNWords(current, c.config.Overlap/4)
				current = overlapWords + " " + word
			} else {
				current = word
			}
		} else {
			current = testText
		}
	}

	// Add remaining text as final chunk
	if current != "" {
		chunk := Chunk{
			ID:         c.generateChunkID(current, baseIndex+chunkIndex),
			Text:       current,
			TokenCount: c.estimateTokens(current),
			Index:      baseIndex + chunkIndex,
			SourceFile: "",
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

// getLastNWords returns the last n words from text
func (c *Chunker) getLastNWords(text string, n int) string {
	words := strings.Fields(text)
	if len(words) <= n {
		return text
	}
	return strings.Join(words[len(words)-n:], " ")
}

// generateChunkID creates a unique ID for a chunk
func (c *Chunker) generateChunkID(text string, index int) string {
	hash := md5.Sum([]byte(text))
	return fmt.Sprintf("%x-%d", hash[:8], index)
}
