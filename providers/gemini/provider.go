package gemini

import (
	"context"
	"fmt"

	"github.com/Yugp72/recurseai/providers"
	"google.golang.org/genai"
)

type GeminiProvider struct {
	client     *genai.Client
	model      string
	embedModel string
	embedTry   []string
	maxTokens  int
}

func NewGeminiProvider(apiKey, model string) (*GeminiProvider, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	embedModel := "gemini-embedding-001"
	embedTry := []string{
		"gemini-embedding-001",
	}

	return &GeminiProvider{
		client:     client,
		model:      model,
		embedModel: embedModel,
		embedTry:   embedTry,
		maxTokens:  8192,
	}, nil
}

func (p *GeminiProvider) Complete(ctx context.Context, req providers.CompletionRequest) (providers.CompletionResponse, error) {
	config := &genai.GenerateContentConfig{}

	if req.Temperature > 0 {
		temp := req.Temperature
		config.Temperature = &temp
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, "system")
	}

	contents := []*genai.Content{
		genai.NewContentFromText(req.UserPrompt, "user"),
	}

	resp, err := p.client.Models.GenerateContent(ctx, p.model, contents, config)
	if err != nil {
		return providers.CompletionResponse{}, fmt.Errorf("gemini completion error: %w", err)
	}

	text := p.extractText(resp)

	tokenUsed := 0
	if resp.UsageMetadata != nil {
		tokenUsed = int(resp.UsageMetadata.TotalTokenCount)
	}

	return providers.CompletionResponse{
		Content:   text,
		TokenUsed: tokenUsed,
		Model:     p.model,
		Provider:  p.Name(),
	}, nil
}

func (p *GeminiProvider) Embed(ctx context.Context, input string) (providers.EmbeddingResult, error) {
	contents := []*genai.Content{
		genai.NewContentFromText(input, "user"),
	}

	models := p.embedTry
	if len(models) == 0 {
		models = []string{p.embedModel}
	}

	var lastErr error
	for _, m := range models {
		resp, err := p.client.Models.EmbedContent(ctx, m, contents, nil)
		if err != nil {
			lastErr = err
			continue
		}

		if len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Values) == 0 {
			lastErr = fmt.Errorf("gemini returned empty embeddings for model %s", m)
			continue
		}

		vector := resp.Embeddings[0].Values
		p.embedModel = m

		return providers.EmbeddingResult{
			Vector:     vector,
			Dimensions: len(vector),
			Model:      m,
		}, nil
	}

	return providers.EmbeddingResult{}, fmt.Errorf("gemini embed error: %w", lastErr)
}

func (p *GeminiProvider) Name() string {
	return "gemini"
}

func (p *GeminiProvider) MaxTokens() int {
	return p.maxTokens
}

func (p *GeminiProvider) extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}
	return resp.Text()
}
