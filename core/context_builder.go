package core

import (
	"fmt"
	"sort"
	"strings"
)

type ContextWindow struct {
	Chunks      []Chunk
	Summaries   []string
	TotalTokens int
	Sources     []string
}

type ContextBuilderConfig struct {
	MaxTokens         int
	IncludeFullChunks bool
	SummaryFallback   bool
}

type ContextBuilder struct {
	config ContextBuilderConfig
}

func NewContextBuilder(cfg ContextBuilderConfig) *ContextBuilder {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4000
	}
	return &ContextBuilder{config: cfg}
}

func (b *ContextBuilder) Build(result TraversalResult) ContextWindow {
	rankedChunks := b.rankByRelevance(result)

	summaries := make([]string, 0, len(result.RelevantNodes))
	seenSummary := make(map[string]bool)
	for _, node := range result.RelevantNodes {
		s := strings.TrimSpace(node.Summary)
		if s == "" || seenSummary[s] {
			continue
		}
		seenSummary[s] = true
		summaries = append(summaries, s)
	}

	if b.config.IncludeFullChunks {
		return b.fitWithinTokenLimit(rankedChunks, summaries)
	}

	if b.config.SummaryFallback {
		return b.fitWithinTokenLimit(nil, summaries)
	}

	return b.fitWithinTokenLimit(rankedChunks, nil)
}

func (b *ContextBuilder) buildPrompt(window ContextWindow, query string) string {
	var sb strings.Builder

	sb.WriteString("You are given retrieved context to answer the user query. Use only relevant details.\n\n")

	if len(window.Summaries) > 0 {
		sb.WriteString("Summaries:\n")
		for i, s := range window.Summaries {
			sb.WriteString(fmt.Sprintf("%d) %s\n", i+1, s))
		}
		sb.WriteString("\n")
	}

	if len(window.Chunks) > 0 {
		sb.WriteString("Chunks:\n")
		for i, c := range window.Chunks {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, strings.TrimSpace(c.Text)))
		}
		sb.WriteString("\n")
	}

	if len(window.Sources) > 0 {
		sb.WriteString("Sources: ")
		sb.WriteString(strings.Join(window.Sources, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Query: ")
	sb.WriteString(query)

	return sb.String()
}

func (b *ContextBuilder) fitWithinTokenLimit(chunks []Chunk, summaries []string) ContextWindow {
	window := ContextWindow{
		Chunks:    make([]Chunk, 0),
		Summaries: make([]string, 0),
		Sources:   make([]string, 0),
	}

	maxTokens := b.config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4000
	}

	seenSource := make(map[string]bool)
	tokenBudget := 0

	for _, s := range summaries {
		t := estimateTextTokens(s)
		if tokenBudget+t > maxTokens {
			break
		}
		window.Summaries = append(window.Summaries, s)
		tokenBudget += t
	}

	for _, c := range chunks {
		t := c.TokenCount
		if t <= 0 {
			t = estimateTextTokens(c.Text)
		}
		if tokenBudget+t > maxTokens {
			break
		}
		window.Chunks = append(window.Chunks, c)
		tokenBudget += t
		if c.SourceFile != "" && !seenSource[c.SourceFile] {
			seenSource[c.SourceFile] = true
			window.Sources = append(window.Sources, c.SourceFile)
		}
	}

	window.TotalTokens = tokenBudget
	return window
}

func (b *ContextBuilder) rankByRelevance(result TraversalResult) []Chunk {
	if len(result.RelevantChunks) == 0 {
		return nil
	}

	nodeScore := result.Scores
	chunkScore := make(map[string]float64, len(result.RelevantChunks))

	for _, node := range result.RelevantNodes {
		score := nodeScore[node.ID]
		for _, cid := range node.ChunkIDs {
			if prev, ok := chunkScore[cid]; !ok || score > prev {
				chunkScore[cid] = score
			}
		}
	}

	ranked := make([]Chunk, len(result.RelevantChunks))
	copy(ranked, result.RelevantChunks)

	sort.SliceStable(ranked, func(i, j int) bool {
		si := chunkScore[ranked[i].ID]
		sj := chunkScore[ranked[j].ID]
		if si == sj {
			return ranked[i].Index < ranked[j].Index
		}
		return si > sj
	})

	return ranked
}

func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
