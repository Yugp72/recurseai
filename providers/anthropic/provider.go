package anthropic

import (
	"context"

	"github.com/Yugp72/recurseai/providers"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicProvider struct {
	client    *anthropic.Client
	model     string
	maxTokens int
}

func NewAnthropicProvider(apiKey string, model string) *AnthropicProvider {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicProvider{
		client:    &client,
		model:     model,
		maxTokens: 4096,
	}
}

func (p *AnthropicProvider) Name() string {

	return "anthropic"
}

func (p *AnthropicProvider) MaxTokens() int {
	return p.maxTokens
}

func (p *AnthropicProvider) Complete(ctx context.Context, req providers.CompletionRequest) (providers.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.UserPrompt)),
	}

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(p.model),
		MaxTokens:   int64(maxTokens),
		Messages:    messages,
		Temperature: anthropic.Opt(float64(temp)),
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	response, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return providers.CompletionResponse{}, err
	}

	var content string
	for _, block := range response.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return providers.CompletionResponse{
		Content:   content,
		TokenUsed: int(response.Usage.InputTokens + response.Usage.OutputTokens),
		Model:     string(response.Model),
		Provider:  p.Name(),
	}, nil
}

func (p *AnthropicProvider) Embed(ctx context.Context, input string) (providers.EmbeddingResult, error) {
	// Anthropic does not provide embedding models
	// This would need to be delegated to another provider (e.g., OpenAI, Voyage AI)
	return providers.EmbeddingResult{}, nil
}
