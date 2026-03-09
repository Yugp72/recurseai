package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Yugp72/recurseai/providers"
	"github.com/Yugp72/recurseai/providers/anthropic"
	"github.com/Yugp72/recurseai/providers/gemini"
	"github.com/Yugp72/recurseai/providers/ollama"
	"github.com/Yugp72/recurseai/providers/openai"
	"gopkg.in/yaml.v3"
)

type ProviderRegistry = providers.ProviderRegistry

type Config struct {
	Ingestion IngestionConfig `json:"ingestion" yaml:"ingestion"`
	Tree      TreeConfig      `json:"tree" yaml:"tree"`
	Query     QueryConfig     `json:"query" yaml:"query"`
	Providers ProvidersConfig `json:"providers" yaml:"providers"`
	Storage   StorageConfig   `json:"storage" yaml:"storage"`
}

type IngestionConfig struct {
	ChunkSize      int `json:"chunkSize" yaml:"chunkSize"`
	ChunkOverlap   int `json:"chunkOverlap" yaml:"chunkOverlap"`
	WorkerPoolSize int `json:"workerPoolSize" yaml:"workerPoolSize"`
}

type TreeConfig struct {
	BranchFactor      int    `json:"branchFactor" yaml:"branchFactor"`
	MaxLevels         int    `json:"maxLevels" yaml:"maxLevels"`
	SummarizeProvider string `json:"summarizeProvider" yaml:"summarizeProvider"`
	SummarizeModel    string `json:"summarizeModel" yaml:"summarizeModel"`
}

type QueryConfig struct {
	BeamWidth           int     `json:"beamWidth" yaml:"beamWidth"`
	SimilarityThreshold float64 `json:"similarityThreshold" yaml:"similarityThreshold"`
	AnswerProvider      string  `json:"answerProvider" yaml:"answerProvider"`
	AnswerModel         string  `json:"answerModel" yaml:"answerModel"`
}

type ProvidersConfig struct {
	OpenAI    ProviderCreds `json:"openai" yaml:"openai"`
	Anthropic ProviderCreds `json:"anthropic" yaml:"anthropic"`
	Gemini    ProviderCreds `json:"gemini" yaml:"gemini"`
	Ollama    OllamaCreds   `json:"ollama" yaml:"ollama"`
}

type ProviderCreds struct {
	APIKey string `json:"apiKey" yaml:"apiKey"`
	Model  string `json:"model" yaml:"model"`
}

type OllamaCreds struct {
	BaseURL string `json:"baseURL" yaml:"baseURL"`
	Model   string `json:"model" yaml:"model"`
}

type StorageConfig struct {
	VectorDB string `json:"vectorDB" yaml:"vectorDB"`
	TreeDB   string `json:"treeDB" yaml:"treeDB"`
}

func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		cfg := LoadFromEnv()
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := LoadFromEnv()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if len(bytesTrimSpace(b)) > 0 {
			if err := yaml.Unmarshal(b, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse yaml config: %w", err)
			}
		}
	case ".json":
		if len(bytesTrimSpace(b)) > 0 {
			if err := json.Unmarshal(b, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse json config: %w", err)
			}
		}
	default:
		if len(bytesTrimSpace(b)) > 0 {
			if err := yaml.Unmarshal(b, cfg); err != nil {
				if err2 := json.Unmarshal(b, cfg); err2 != nil {
					return nil, fmt.Errorf("failed to parse config (tried yaml/json): %w", err)
				}
			}
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func LoadFromEnv() *Config {
	c := &Config{
		Ingestion: IngestionConfig{
			ChunkSize:      512,
			ChunkOverlap:   64,
			WorkerPoolSize: 4,
		},
		Tree: TreeConfig{
			BranchFactor:      4,
			MaxLevels:         8,
			SummarizeProvider: "openai",
		},
		Query: QueryConfig{
			BeamWidth:           3,
			SimilarityThreshold: 0.2,
			AnswerProvider:      "openai",
		},
		Storage: StorageConfig{
			VectorDB: "recurseai.db",
			TreeDB:   "recurseai.db",
		},
	}

	c.Ingestion.ChunkSize = envInt("RECURSEAI_INGEST_CHUNK_SIZE", c.Ingestion.ChunkSize)
	c.Ingestion.ChunkOverlap = envInt("RECURSEAI_INGEST_CHUNK_OVERLAP", c.Ingestion.ChunkOverlap)
	c.Ingestion.WorkerPoolSize = envInt("RECURSEAI_INGEST_WORKERS", c.Ingestion.WorkerPoolSize)

	c.Tree.BranchFactor = envInt("RECURSEAI_TREE_BRANCH_FACTOR", c.Tree.BranchFactor)
	c.Tree.MaxLevels = envInt("RECURSEAI_TREE_MAX_LEVELS", c.Tree.MaxLevels)
	c.Tree.SummarizeProvider = envString("RECURSEAI_TREE_PROVIDER", c.Tree.SummarizeProvider)
	c.Tree.SummarizeModel = envString("RECURSEAI_TREE_MODEL", c.Tree.SummarizeModel)

	c.Query.BeamWidth = envInt("RECURSEAI_QUERY_BEAM_WIDTH", c.Query.BeamWidth)
	c.Query.SimilarityThreshold = envFloat("RECURSEAI_QUERY_SIMILARITY_THRESHOLD", c.Query.SimilarityThreshold)
	c.Query.AnswerProvider = envString("RECURSEAI_QUERY_PROVIDER", c.Query.AnswerProvider)
	c.Query.AnswerModel = envString("RECURSEAI_QUERY_MODEL", c.Query.AnswerModel)

	c.Providers.OpenAI.APIKey = envString("OPENAI_API_KEY", c.Providers.OpenAI.APIKey)
	c.Providers.OpenAI.Model = envString("OPENAI_MODEL", c.Providers.OpenAI.Model)
	if c.Providers.OpenAI.Model == "" {
		c.Providers.OpenAI.Model = "gpt-4o-mini"
	}

	c.Providers.Anthropic.APIKey = envString("ANTHROPIC_API_KEY", c.Providers.Anthropic.APIKey)
	c.Providers.Anthropic.Model = envString("ANTHROPIC_MODEL", c.Providers.Anthropic.Model)
	if c.Providers.Anthropic.Model == "" {
		c.Providers.Anthropic.Model = "claude-3-5-haiku-latest"
	}

	c.Providers.Gemini.APIKey = envString("GEMINI_API_KEY", c.Providers.Gemini.APIKey)
	c.Providers.Gemini.Model = envString("GEMINI_MODEL", c.Providers.Gemini.Model)
	if c.Providers.Gemini.Model == "" {
		c.Providers.Gemini.Model = "gemini-2.0-flash"
	}

	c.Providers.Ollama.BaseURL = envString("OLLAMA_BASE_URL", c.Providers.Ollama.BaseURL)
	if c.Providers.Ollama.BaseURL == "" {
		c.Providers.Ollama.BaseURL = "http://localhost:11434"
	}
	c.Providers.Ollama.Model = envString("OLLAMA_MODEL", c.Providers.Ollama.Model)
	if c.Providers.Ollama.Model == "" {
		c.Providers.Ollama.Model = "llama3.1"
	}

	c.Storage.VectorDB = envString("RECURSEAI_VECTOR_DB", c.Storage.VectorDB)
	c.Storage.TreeDB = envString("RECURSEAI_TREE_DB", c.Storage.TreeDB)

	return c
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Ingestion.ChunkSize <= 0 {
		return errors.New("ingestion.chunkSize must be > 0")
	}
	if c.Ingestion.ChunkOverlap < 0 {
		return errors.New("ingestion.chunkOverlap must be >= 0")
	}
	if c.Ingestion.WorkerPoolSize <= 0 {
		return errors.New("ingestion.workerPoolSize must be > 0")
	}
	if c.Tree.BranchFactor <= 0 {
		return errors.New("tree.branchFactor must be > 0")
	}
	if c.Tree.MaxLevels <= 0 {
		return errors.New("tree.maxLevels must be > 0")
	}
	if c.Query.BeamWidth <= 0 {
		return errors.New("query.beamWidth must be > 0")
	}
	if c.Query.SimilarityThreshold < -1 || c.Query.SimilarityThreshold > 1 {
		return errors.New("query.similarityThreshold must be between -1 and 1")
	}
	if strings.TrimSpace(c.Storage.VectorDB) == "" {
		return errors.New("storage.vectorDB is required")
	}
	if strings.TrimSpace(c.Storage.TreeDB) == "" {
		return errors.New("storage.treeDB is required")
	}
	return nil
}

func (c *Config) BuildRegistry() (*ProviderRegistry, error) {
	if c == nil {
		return nil, errors.New("config is nil")
	}

	r := providers.NewProviderRegistry()

	if c.Providers.OpenAI.APIKey != "" {
		opModel := c.Providers.OpenAI.Model
		if c.Query.AnswerProvider == "openai" && c.Query.AnswerModel != "" {
			opModel = c.Query.AnswerModel
		} else if c.Tree.SummarizeProvider == "openai" && c.Tree.SummarizeModel != "" {
			opModel = c.Tree.SummarizeModel
		}
		r.Register("openai", openai.NewOpenAIProvider(c.Providers.OpenAI.APIKey, opModel, "text-embedding-3-small", 4096))
	}

	if c.Providers.Anthropic.APIKey != "" {
		anModel := c.Providers.Anthropic.Model
		if c.Query.AnswerProvider == "anthropic" && c.Query.AnswerModel != "" {
			anModel = c.Query.AnswerModel
		} else if c.Tree.SummarizeProvider == "anthropic" && c.Tree.SummarizeModel != "" {
			anModel = c.Tree.SummarizeModel
		}
		r.Register("anthropic", anthropic.NewAnthropicProvider(c.Providers.Anthropic.APIKey, anModel))
	}

	if c.Providers.Gemini.APIKey != "" {
		gmModel := c.Providers.Gemini.Model
		if c.Query.AnswerProvider == "gemini" && c.Query.AnswerModel != "" {
			gmModel = c.Query.AnswerModel
		} else if c.Tree.SummarizeProvider == "gemini" && c.Tree.SummarizeModel != "" {
			gmModel = c.Tree.SummarizeModel
		}
		gp, err := gemini.NewGeminiProvider(c.Providers.Gemini.APIKey, gmModel)
		if err != nil {
			return nil, err
		}
		r.Register("gemini", gp)
	}

	if c.Providers.Ollama.Model != "" {
		oModel := c.Providers.Ollama.Model
		if c.Query.AnswerProvider == "ollama" && c.Query.AnswerModel != "" {
			oModel = c.Query.AnswerModel
		} else if c.Tree.SummarizeProvider == "ollama" && c.Tree.SummarizeModel != "" {
			oModel = c.Tree.SummarizeModel
		}
		r.Register("ollama", ollama.NewOllamaProvider(c.Providers.Ollama.BaseURL, oModel))
	}

	if len(r.List()) == 0 {
		return nil, errors.New("no providers configured")
	}

	preferredDefault := c.Query.AnswerProvider
	if preferredDefault == "" {
		preferredDefault = c.Tree.SummarizeProvider
	}
	if preferredDefault != "" {
		_ = r.SetDefault(preferredDefault)
	}

	return r, nil
}

func envString(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
