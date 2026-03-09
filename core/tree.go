package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Yugp72/recurseai/providers"
)

type LLMProvider = providers.LLMProvider

// TreeNode represents one node in the recursive summary tree.
type TreeNode struct {
	ID         string
	Level      int
	Summary    string
	Embedding  []float32
	Children   []*TreeNode
	ChunkIDs   []string // leaf nodes reference original chunks
	TokenCount int
	Provider   string // which model built this node
	CreatedAt  time.Time
}

// Tree is the hierarchical representation for a document.
type Tree struct {
	Root        *TreeNode
	TotalLevels int
	NodeCount   int
	DocID       string
}

// BuildConfig controls how the tree is generated.
type BuildConfig struct {
	BranchFactor      int
	MaxLevels         int
	SummarizeProvider string
	SummarizePrompt   string
}

func NewTree(docID string) *Tree {
	return &Tree{DocID: docID}
}

func BuildTree(ctx context.Context, chunks []Chunk, provider LLMProvider, cfg BuildConfig) (*Tree, error) {
	if provider == nil {
		return nil, errors.New("provider is required")
	}
	if len(chunks) == 0 {
		return nil, errors.New("at least one chunk is required")
	}
	if cfg.BranchFactor <= 0 {
		cfg.BranchFactor = 4
	}
	if cfg.MaxLevels <= 0 {
		cfg.MaxLevels = 8
	}

	now := time.Now().UTC()
	leafNodes := make([]*TreeNode, 0, len(chunks))
	for _, ch := range chunks {
		emb := make([]float32, 0)
		embedRes, err := provider.Embed(ctx, ch.Text)
		if err == nil {
			emb = embedRes.Vector
		}

		leaf := &TreeNode{
			ID:         generateNodeID(),
			Level:      0,
			Summary:    ch.Text,
			Embedding:  emb,
			Children:   nil,
			ChunkIDs:   []string{ch.ID},
			TokenCount: ch.TokenCount,
			Provider:   provider.Name(),
			CreatedAt:  now,
		}
		leafNodes = append(leafNodes, leaf)
	}

	tree := NewTree("")
	allNodes := len(leafNodes)
	current := leafNodes
	level := 0

	for len(current) > 1 && level < cfg.MaxLevels {
		parents, err := buildLevel(ctx, current, provider, cfg)
		if err != nil {
			return nil, err
		}
		if len(parents) == len(current) {
			break
		}
		allNodes += len(parents)
		current = parents
		level++
	}

	tree.Root = current[0]
	tree.TotalLevels = tree.Root.Level + 1
	tree.NodeCount = allNodes

	return tree, nil
}

func buildLevel(ctx context.Context, nodes []*TreeNode, provider LLMProvider, cfg BuildConfig) ([]*TreeNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	if len(nodes) == 1 {
		return nodes, nil
	}

	branchFactor := cfg.BranchFactor
	if branchFactor <= 1 {
		branchFactor = 2
	}

	parents := make([]*TreeNode, 0, (len(nodes)+branchFactor-1)/branchFactor)
	for i := 0; i < len(nodes); i += branchFactor {
		end := i + branchFactor
		if end > len(nodes) {
			end = len(nodes)
		}
		parent, err := buildParentNode(ctx, nodes[i:end], provider)
		if err != nil {
			return nil, err
		}
		parents = append(parents, parent)
	}

	return parents, nil
}

func buildParentNode(ctx context.Context, children []*TreeNode, provider LLMProvider) (*TreeNode, error) {
	if len(children) == 0 {
		return nil, errors.New("children are required")
	}

	summary, err := summarizeChildren(ctx, children, provider)
	if err != nil {
		return nil, err
	}

	tokenCount := 0
	chunkIDs := make([]string, 0)
	for _, ch := range children {
		tokenCount += ch.TokenCount
		chunkIDs = append(chunkIDs, ch.ChunkIDs...)
	}

	emb := make([]float32, 0)
	embedRes, err := provider.Embed(ctx, summary)
	if err == nil {
		emb = embedRes.Vector
	}

	return &TreeNode{
		ID:         generateNodeID(),
		Level:      children[0].Level + 1,
		Summary:    summary,
		Embedding:  emb,
		Children:   children,
		ChunkIDs:   chunkIDs,
		TokenCount: tokenCount,
		Provider:   provider.Name(),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func summarizeChildren(ctx context.Context, children []*TreeNode, provider LLMProvider) (string, error) {
	if len(children) == 0 {
		return "", errors.New("children are required")
	}

	var builder strings.Builder
	builder.WriteString("Summarize the following content into a concise, faithful parent summary:\n\n")
	for i, ch := range children {
		builder.WriteString(fmt.Sprintf("Section %d:\n%s\n\n", i+1, ch.Summary))
	}

	resp, err := provider.Complete(ctx, providers.CompletionRequest{
		SystemPrompt: "You build hierarchical summaries for retrieval.",
		UserPrompt:   builder.String(),
		MaxTokens:    512,
		Temperature:  0.1,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}

func generateNodeID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	return "node-" + hex.EncodeToString(b)
}
