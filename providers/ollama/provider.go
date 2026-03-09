package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Yugp72/recurseai/providers"
)

type OllamaProvider struct {
	baseURL    string
	model      string
	embedModel string
	httpClient *http.Client
}

type ollamaRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	System  string `json:"system,omitempty"`
	Stream  bool   `json:"stream"`
	Options struct {
		Temperature float32 `json:"temperature,omitempty"`
		NumPredict  int     `json:"num_predict,omitempty"`
	} `json:"options,omitempty"`
}

type ollamaResponse struct {
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	EvalCount int    `json:"eval_count"`
}

type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	embedModel := model
	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		embedModel: embedModel,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Local models can be slow
		},
	}
}

func (p *OllamaProvider) Complete(ctx context.Context, req providers.CompletionRequest) (providers.CompletionResponse, error) {
	ollamaReq := ollamaRequest{
		Model:  p.model,
		System: req.SystemPrompt,
		Prompt: req.UserPrompt,
		Stream: false,
	}

	if req.Temperature > 0 {
		ollamaReq.Options.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		ollamaReq.Options.NumPredict = req.MaxTokens
	}

	body, err := p.post(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return providers.CompletionResponse{}, fmt.Errorf("ollama generate error: %w", err)
	}

	var resp ollamaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.CompletionResponse{}, fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return providers.CompletionResponse{
		Content:   resp.Response,
		TokenUsed: resp.EvalCount,
		Model:     p.model,
		Provider:  p.Name(),
	}, nil
}

func (p *OllamaProvider) Embed(ctx context.Context, input string) (providers.EmbeddingResult, error) {
	req := ollamaEmbeddingRequest{
		Model:  p.embedModel,
		Prompt: input,
	}

	body, err := p.post(ctx, "/api/embeddings", req)
	if err != nil {
		return providers.EmbeddingResult{}, fmt.Errorf("ollama embeddings error: %w", err)
	}

	var resp ollamaEmbeddingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.EmbeddingResult{}, fmt.Errorf("failed to parse ollama embedding response: %w", err)
	}

	return providers.EmbeddingResult{
		Vector:     resp.Embedding,
		Dimensions: len(resp.Embedding),
		Model:      p.embedModel,
	}, nil
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) MaxTokens() int {
	return 4096
}

func (p *OllamaProvider) post(ctx context.Context, endpoint string, body any) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s%s", p.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ollama (is it running?): %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
