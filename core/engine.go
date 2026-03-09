package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Yugp72/recurseai/providers"
)

type ProviderRegistry = providers.ProviderRegistry

type TreeStore interface {
	SaveTree(ctx context.Context, tree *Tree) error
	GetLatestTree(ctx context.Context) (*Tree, error)
	ListTrees(ctx context.Context) ([]string, error)
}

type Engine struct {
	registry       *ProviderRegistry
	chunker        *Chunker
	store          VectorStore
	treeStore      TreeStore
	contextBuilder *ContextBuilder
	config         EngineConfig
}

type EngineConfig struct {
	IngestWorkers     int
	TreeBuildProvider string
	QueryProvider     string
	ChunkerConfig     ChunkerConfig
	TreeBuildConfig   BuildConfig
	TraversalConfig   TraversalConfig
	ContextConfig     ContextBuilderConfig
}

type IngestResult struct {
	DocID      string
	ChunkCount int
	NodeCount  int
	TimeTaken  time.Duration
}

type QueryResult struct {
	Answer     string
	Sources    []Chunk
	TokensUsed int
	Provider   string
	TimeTaken  time.Duration
}

func NewEngine(registry *ProviderRegistry, store VectorStore, cfg EngineConfig) *Engine {
	if cfg.IngestWorkers <= 0 {
		cfg.IngestWorkers = 4
	}

	e := &Engine{
		registry:       registry,
		chunker:        NewChunker(cfg.ChunkerConfig),
		store:          store,
		contextBuilder: NewContextBuilder(cfg.ContextConfig),
		config:         cfg,
	}

	if ts, ok := any(store).(TreeStore); ok {
		e.treeStore = ts
	}

	return e
}

func (e *Engine) Ingest(ctx context.Context, filePath string) (IngestResult, error) {
	start := time.Now()
	if e == nil {
		return IngestResult{}, errors.New("engine is nil")
	}
	if e.chunker == nil {
		return IngestResult{}, errors.New("chunker is not configured")
	}
	if e.store == nil {
		return IngestResult{}, errors.New("vector store is not configured")
	}

	chunks, err := e.chunker.ChunkFile(filePath)
	if err != nil {
		return IngestResult{}, err
	}
	if len(chunks) == 0 {
		return IngestResult{}, errors.New("no chunks produced from file")
	}

	if err := e.ingestChunks(ctx, chunks); err != nil {
		return IngestResult{}, err
	}

	tree, err := e.buildAndStoreTree(ctx, chunks)
	if err != nil {
		return IngestResult{}, err
	}

	if tree.DocID == "" {
		tree.DocID = generateDocID(filePath)
	}

	return IngestResult{
		DocID:      tree.DocID,
		ChunkCount: len(chunks),
		NodeCount:  tree.NodeCount,
		TimeTaken:  time.Since(start),
	}, nil
}

func (e *Engine) Query(ctx context.Context, question string) (QueryResult, error) {
	return e.queryWithProvider(ctx, question, "")
}

func (e *Engine) QueryWithProvider(ctx context.Context, question, providerName string) (QueryResult, error) {
	return e.queryWithProvider(ctx, question, providerName)
}

func (e *Engine) ListDocs(ctx context.Context) ([]string, error) {
	if e == nil {
		return nil, errors.New("engine is nil")
	}
	if e.treeStore == nil {
		return nil, errors.New("tree store is not configured")
	}
	return e.treeStore.ListTrees(ctx)
}

func (e *Engine) queryWithProvider(ctx context.Context, question, providerName string) (QueryResult, error) {
	start := time.Now()
	if e == nil {
		return QueryResult{}, errors.New("engine is nil")
	}
	if e.registry == nil {
		return QueryResult{}, errors.New("provider registry is not configured")
	}
	if e.contextBuilder == nil {
		return QueryResult{}, errors.New("context builder is not configured")
	}
	if e.treeStore == nil {
		return QueryResult{}, errors.New("tree store is not configured")
	}

	selectedProvider := providerName
	if selectedProvider == "" {
		selectedProvider = e.config.QueryProvider
	}

	provider, err := e.getProvider(selectedProvider)
	if err != nil {
		return QueryResult{}, err
	}

	tree, err := e.treeStore.GetLatestTree(ctx)
	if err != nil {
		return QueryResult{}, err
	}
	if tree == nil || tree.Root == nil {
		return QueryResult{}, errors.New("no tree available for querying")
	}

	qEmb, err := provider.Embed(ctx, question)
	if err != nil {
		return QueryResult{}, err
	}

	traverser := NewTraverser(tree, e.store, e.config.TraversalConfig)
	travResult, err := traverser.Traverse(ctx, qEmb.Vector)
	if err != nil {
		return QueryResult{}, err
	}

	window := e.contextBuilder.Build(travResult)
	prompt := e.contextBuilder.buildPrompt(window, question)

	resp, err := provider.Complete(ctx, providers.CompletionRequest{
		SystemPrompt: "Answer using the provided context. If uncertain, say so.",
		UserPrompt:   prompt,
		MaxTokens:    minInt(1024, provider.MaxTokens()),
		Temperature:  0.2,
	})
	if err != nil {
		return QueryResult{}, err
	}

	return QueryResult{
		Answer:     resp.Content,
		Sources:    window.Chunks,
		TokensUsed: resp.TokenUsed,
		Provider:   provider.Name(),
		TimeTaken:  time.Since(start),
	}, nil
}

func (e *Engine) ingestChunks(ctx context.Context, chunks []Chunk) error {
	if e.store == nil {
		return errors.New("vector store is not configured")
	}
	if len(chunks) == 0 {
		return nil
	}

	workers := e.config.IngestWorkers
	if workers <= 0 {
		workers = 4
	}

	in := make(chan Chunk)
	out := make(chan Chunk)
	errCh := make(chan error, 1)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range in {
				embedded, err := e.embedChunk(ctx, ch)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					continue
				}
				select {
				case out <- embedded:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(in)
		for _, ch := range chunks {
			select {
			case in <- ch:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	processed := make([]Chunk, 0, len(chunks))
	for ch := range out {
		processed = append(processed, ch)
	}

	select {
	case err := <-errCh:
		return err
	default:
	}

	if len(processed) == 0 {
		return errors.New("no chunks were processed")
	}

	return e.store.UpsertChunks(ctx, processed)
}

func (e *Engine) embedChunk(ctx context.Context, chunk Chunk) (Chunk, error) {
	provider, err := e.getProvider(e.config.TreeBuildProvider)
	if err != nil {
		return Chunk{}, err
	}

	if chunk.ID == "" {
		chunk.ID = generateDocID(chunk.Text)
	}
	if chunk.TokenCount <= 0 {
		chunk.TokenCount = estimateTextTokens(chunk.Text)
	}

	// Ensure embedding can be produced for this chunk. The resulting vector is
	// intentionally not stored on Chunk because vector persistence is delegated to VectorStore.
	_, err = provider.Embed(ctx, chunk.Text)
	if err != nil {
		return Chunk{}, err
	}

	return chunk, nil
}

func (e *Engine) buildAndStoreTree(ctx context.Context, chunks []Chunk) (*Tree, error) {
	provider, err := e.getProvider(e.config.TreeBuildProvider)
	if err != nil {
		return nil, err
	}

	tree, err := BuildTree(ctx, chunks, provider, e.config.TreeBuildConfig)
	if err != nil {
		return nil, err
	}

	if tree.DocID == "" {
		src := ""
		if len(chunks) > 0 {
			src = chunks[0].SourceFile
		}
		tree.DocID = generateDocID(src)
	}

	if e.treeStore != nil {
		if err := e.treeStore.SaveTree(ctx, tree); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func (e *Engine) getProvider(name string) (providers.LLMProvider, error) {
	if e.registry == nil {
		return nil, errors.New("provider registry is nil")
	}
	if name != "" {
		if p, ok := e.registry.Get(name); ok {
			return p, nil
		}
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	if p, ok := e.registry.GetDefault(); ok {
		return p, nil
	}
	return nil, errors.New("no default provider configured")
}

func generateDocID(seed string) string {
	base := filepath.Base(seed)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "doc"
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(b))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
