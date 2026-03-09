package core

import (
	"context"
	"errors"
	"math"
	"sort"
)

// VectorStore is an optional chunk source used during traversal.
// Implementations can provide canonical chunk payloads by IDs.
type VectorStore interface {
	UpsertChunks(ctx context.Context, chunks []Chunk) error
	SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]Chunk, error)
	GetChunksByIDs(ctx context.Context, ids []string) ([]Chunk, error)
}

type TraversalConfig struct {
	BeamWidth           int
	SimilarityThreshold float64
	MaxDepth            int
}

type TraversalResult struct {
	RelevantNodes  []*TreeNode
	RelevantChunks []Chunk
	Scores         map[string]float64 // nodeID -> score
	LevelsSearched int
}

type Traverser struct {
	tree   *Tree
	store  VectorStore
	config TraversalConfig
}

func NewTraverser(tree *Tree, store VectorStore, cfg TraversalConfig) *Traverser {
	if cfg.BeamWidth <= 0 {
		cfg.BeamWidth = 3
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.2
	}
	if cfg.MaxDepth <= 0 {
		if tree != nil && tree.TotalLevels > 0 {
			cfg.MaxDepth = tree.TotalLevels
		} else {
			cfg.MaxDepth = 8
		}
	}

	return &Traverser{
		tree:   tree,
		store:  store,
		config: cfg,
	}
}

func (t *Traverser) Traverse(ctx context.Context, queryEmbedding []float32) (TraversalResult, error) {
	if t == nil || t.tree == nil || t.tree.Root == nil {
		return TraversalResult{}, errors.New("tree/root is required")
	}
	if len(queryEmbedding) == 0 {
		return TraversalResult{}, errors.New("query embedding is required")
	}

	result := TraversalResult{
		RelevantNodes:  make([]*TreeNode, 0),
		RelevantChunks: make([]Chunk, 0),
		Scores:         make(map[string]float64),
	}

	visited := make(map[string]bool)
	current := []*TreeNode{t.tree.Root}
	for depth := 0; depth < t.config.MaxDepth && len(current) > 0; depth++ {
		levelScores := t.scoreNodes(ctx, current, queryEmbedding)
		for id, score := range levelScores {
			result.Scores[id] = score
		}

		nextNodes, err := t.traverseLevel(ctx, current, queryEmbedding)
		if err != nil {
			return TraversalResult{}, err
		}
		if len(nextNodes) == 0 {
			break
		}

		nextChildren := make([]*TreeNode, 0)
		for _, n := range nextNodes {
			if !visited[n.ID] {
				visited[n.ID] = true
				result.RelevantNodes = append(result.RelevantNodes, n)
			}
			if len(n.Children) > 0 {
				nextChildren = append(nextChildren, n.Children...)
			}
		}

		result.LevelsSearched++
		current = nextChildren
	}

	result.RelevantChunks = t.collectRelevantChunks(ctx, result.RelevantNodes)
	return result, nil
}

func (t *Traverser) traverseLevel(ctx context.Context, nodes []*TreeNode, queryVec []float32) ([]*TreeNode, error) {
	_ = ctx
	if len(nodes) == 0 {
		return nil, nil
	}

	scores := t.scoreNodes(ctx, nodes, queryVec)
	selectedIDs := t.pruneByThreshold(scores)
	if len(selectedIDs) == 0 {
		return nil, nil
	}

	nodeByID := make(map[string]*TreeNode, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}

	selected := make([]*TreeNode, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if node, ok := nodeByID[id]; ok {
			selected = append(selected, node)
		}
	}

	return selected, nil
}

func (t *Traverser) scoreNodes(ctx context.Context, nodes []*TreeNode, queryVec []float32) map[string]float64 {
	_ = ctx
	scores := make(map[string]float64, len(nodes))
	if len(queryVec) == 0 {
		for _, n := range nodes {
			scores[n.ID] = 0
		}
		return scores
	}

	for _, n := range nodes {
		s := cosineSimilarity(queryVec, n.Embedding)
		scores[n.ID] = s
	}

	return scores
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	var dot float64
	var normA float64
	var normB float64
	for i := 0; i < minLen; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (t *Traverser) pruneByThreshold(scores map[string]float64) []string {
	if len(scores) == 0 {
		return nil
	}

	type pair struct {
		id    string
		score float64
	}
	pairs := make([]pair, 0, len(scores))
	for id, score := range scores {
		if score >= t.config.SimilarityThreshold {
			pairs = append(pairs, pair{id: id, score: score})
		}
	}

	// Fallback: if nothing meets the threshold, keep best candidates anyway.
	// This prevents empty retrieval for small trees / broad queries.
	if len(pairs) == 0 {
		for id, score := range scores {
			pairs = append(pairs, pair{id: id, score: score})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	if t.config.BeamWidth > 0 && len(pairs) > t.config.BeamWidth {
		pairs = pairs[:t.config.BeamWidth]
	}

	ids := make([]string, 0, len(pairs))
	for _, p := range pairs {
		ids = append(ids, p.id)
	}
	return ids
}

func (t *Traverser) collectRelevantChunks(ctx context.Context, nodes []*TreeNode) []Chunk {
	chunkIDSet := make(map[string]bool)
	chunkIDs := make([]string, 0)

	for _, n := range nodes {
		if len(n.Children) != 0 {
			continue
		}
		for _, cid := range n.ChunkIDs {
			if !chunkIDSet[cid] {
				chunkIDSet[cid] = true
				chunkIDs = append(chunkIDs, cid)
			}
		}
	}

	if len(chunkIDs) == 0 {
		return nil
	}

	if t.store != nil {
		if chunks, err := t.store.GetChunksByIDs(ctx, chunkIDs); err == nil && len(chunks) > 0 {
			return chunks
		}
	}

	leafByChunkID := make(map[string]*TreeNode)
	for _, n := range nodes {
		if len(n.Children) != 0 {
			continue
		}
		for _, cid := range n.ChunkIDs {
			leafByChunkID[cid] = n
		}
	}

	fallback := make([]Chunk, 0, len(chunkIDs))
	for _, cid := range chunkIDs {
		n := leafByChunkID[cid]
		if n == nil {
			continue
		}
		fallback = append(fallback, Chunk{
			ID:         cid,
			Text:       n.Summary,
			TokenCount: n.TokenCount,
			Index:      -1,
		})
	}

	return fallback
}
