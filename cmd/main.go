package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/Yugp72/recurseai/api"
	"github.com/Yugp72/recurseai/config"
	"github.com/Yugp72/recurseai/core"
	storepkg "github.com/Yugp72/recurseai/store"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "recurseai",
	Short: "Recursive retrieval + answering CLI",
}

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingest a document",
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		provider, _ := cmd.Flags().GetString("provider")
		configPath, _ := cmd.Flags().GetString("config")

		if filePath == "" {
			return fmt.Errorf("--file is required")
		}

		engine, err := initEngine(configPath)
		if err != nil {
			return err
		}

		if provider != "" {
			// Provider override applies to tree build + query defaults.
			_ = provider
		}

		res, err := engine.Ingest(context.Background(), filePath)
		if err != nil {
			return err
		}

		fmt.Printf("Ingested doc=%s chunks=%d nodes=%d took=%s\n", res.DocID, res.ChunkCount, res.NodeCount, res.TimeTaken)
		return nil
	},
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query indexed documents",
	RunE: func(cmd *cobra.Command, args []string) error {
		question, _ := cmd.Flags().GetString("question")
		_, _ = cmd.Flags().GetString("doc-id")
		provider, _ := cmd.Flags().GetString("provider")
		configPath, _ := cmd.Flags().GetString("config")

		if question == "" {
			return fmt.Errorf("--question is required")
		}

		engine, err := initEngine(configPath)
		if err != nil {
			return err
		}

		var res core.QueryResult
		if provider != "" {
			res, err = engine.QueryWithProvider(context.Background(), question, provider)
		} else {
			res, err = engine.Query(context.Background(), question)
		}
		if err != nil {
			return err
		}

		fmt.Printf("Answer:\n%s\n\nProvider: %s\nTokens: %d\n", res.Answer, res.Provider, res.TokensUsed)
		if len(res.Sources) > 0 {
			fmt.Println("Sources:")
			for _, s := range res.Sources {
				if s.SourceFile != "" {
					fmt.Printf("- %s (%s)\n", s.ID, s.SourceFile)
				} else {
					fmt.Printf("- %s\n", s.ID)
				}
			}
		}
		return nil
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run HTTP API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		configPath, _ := cmd.Flags().GetString("config")

		engine, err := initEngine(configPath)
		if err != nil {
			return err
		}

		srv := api.NewServer(engine, port)
		fmt.Printf("Starting server on :%d\n", port)
		return srv.Start()
	},
}

func init() {
	ingestCmd.Flags().String("file", "", "path to file to ingest")
	ingestCmd.Flags().String("provider", "", "provider override")
	ingestCmd.Flags().String("config", "recurseai.yaml", "config path")

	queryCmd.Flags().String("question", "", "question to ask")
	queryCmd.Flags().String("doc-id", "", "optional doc ID")
	queryCmd.Flags().String("provider", "", "provider override")
	queryCmd.Flags().String("config", "recurseai.yaml", "config path")

	serveCmd.Flags().Int("port", 8080, "server port")
	serveCmd.Flags().String("config", "recurseai.yaml", "config path")

	rootCmd.AddCommand(ingestCmd, queryCmd, serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initEngine(configPath string) (*core.Engine, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	registry, err := cfg.BuildRegistry()
	if err != nil {
		return nil, err
	}

	vectorStore, err := storepkg.NewSQLiteVectorStore(cfg.Storage.VectorDB)
	if err != nil {
		return nil, err
	}

	treeStore, err := storepkg.NewSQLiteTreeStore(cfg.Storage.TreeDB)
	if err != nil {
		return nil, err
	}

	storeAdapter := &vectorStoreAdapter{
		backend: vectorStore,
		chunks:  make(map[string]core.Chunk),
	}

	engineCfg := core.EngineConfig{
		IngestWorkers:     cfg.Ingestion.WorkerPoolSize,
		TreeBuildProvider: cfg.Tree.SummarizeProvider,
		QueryProvider:     cfg.Query.AnswerProvider,
		ChunkerConfig: core.ChunkerConfig{
			ChunkSize: cfg.Ingestion.ChunkSize,
			Overlap:   cfg.Ingestion.ChunkOverlap,
		},
		TreeBuildConfig: core.BuildConfig{
			BranchFactor:      cfg.Tree.BranchFactor,
			MaxLevels:         cfg.Tree.MaxLevels,
			SummarizeProvider: cfg.Tree.SummarizeProvider,
			SummarizePrompt:   "",
		},
		TraversalConfig: core.TraversalConfig{
			BeamWidth:           cfg.Query.BeamWidth,
			SimilarityThreshold: cfg.Query.SimilarityThreshold,
			MaxDepth:            cfg.Tree.MaxLevels,
		},
		ContextConfig: core.ContextBuilderConfig{
			MaxTokens:         4000,
			IncludeFullChunks: true,
			SummaryFallback:   true,
		},
	}

	engine := core.NewEngine(registry, storeAdapter, engineCfg)
	engineWithTreeStore := core.NewEngine(registry, &combinedStore{VectorStore: storeAdapter, tree: treeStore}, engineCfg)
	_ = engine
	return engineWithTreeStore, nil
}

type combinedStore struct {
	core.VectorStore
	tree *storepkg.SQLiteTreeStore
}

func (c *combinedStore) SaveTree(ctx context.Context, tree *core.Tree) error {
	return c.tree.SaveTree(ctx, tree)
}

func (c *combinedStore) GetLatestTree(ctx context.Context) (*core.Tree, error) {
	return c.tree.GetLatestTree(ctx)
}

func (c *combinedStore) ListTrees(ctx context.Context) ([]string, error) {
	return c.tree.ListTrees(ctx)
}

type vectorStoreAdapter struct {
	backend *storepkg.SQLiteVectorStore
	mu      sync.RWMutex
	chunks  map[string]core.Chunk
}

func (v *vectorStoreAdapter) UpsertChunks(ctx context.Context, chunks []core.Chunk) error {
	if v.backend == nil {
		return fmt.Errorf("vector backend is nil")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	for _, ch := range chunks {
		v.chunks[ch.ID] = ch
		meta := map[string]string{
			"text":   ch.Text,
			"source": ch.SourceFile,
			"token":  strconv.Itoa(ch.TokenCount),
			"index":  strconv.Itoa(ch.Index),
		}
		if err := v.backend.Save(ctx, ch.ID, []float32{0}, meta); err != nil {
			return err
		}
	}
	return nil
}

func (v *vectorStoreAdapter) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]core.Chunk, error) {
	if v.backend == nil {
		return nil, fmt.Errorf("vector backend is nil")
	}
	results, err := v.backend.Search(ctx, embedding, limit)
	if err != nil {
		return nil, err
	}

	chunks := make([]core.Chunk, 0, len(results))
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, r := range results {
		if ch, ok := v.chunks[r.ID]; ok {
			chunks = append(chunks, ch)
		}
	}
	return chunks, nil
}

func (v *vectorStoreAdapter) GetChunksByIDs(ctx context.Context, ids []string) ([]core.Chunk, error) {
	_ = ctx
	v.mu.RLock()
	defer v.mu.RUnlock()

	out := make([]core.Chunk, 0, len(ids))
	for _, id := range ids {
		if ch, ok := v.chunks[id]; ok {
			out = append(out, ch)
		}
	}
	return out, nil
}
