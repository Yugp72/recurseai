package openai

import (
	"context"
	"github.com/recurseai/recurseai/providers"
	"github.com/sashabaranov/go-openai"
)

type OpenAIProvider struct {
	client     *openai.Client
	model      string
	embedModel string
	maxTokens  int
}

func NewOpenAIProvider(apiKey, model, embedModel string, maxTokens int) *OpenAIProvider {
	client := openai.NewClient(apiKey)
	return &OpenAIProvider{
		client:     client,
		model:      model,
		embedModel: embedModel,
		maxTokens:  maxTokens,
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) MaxTokens() int {
	return p.maxTokens
}

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	// Implementation for completion using OpenAI API

	return CompletionResponse{}, nil
}
