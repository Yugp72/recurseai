package openai

import (
	"context"

	"github.com/Yugp72/recurseai/providers"
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

func (p *OpenAIProvider) Complete(ctx context.Context, req providers.CompletionRequest) (providers.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.7
	}

	messages := p.buildMessages(req.SystemPrompt, req.UserPrompt)

	chatReq := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temp,
	}

	response, err := p.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return providers.CompletionResponse{}, err
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
	}

	return providers.CompletionResponse{
		Content:   content,
		TokenUsed: response.Usage.TotalTokens,
		Model:     response.Model,
		Provider:  p.Name(),
	}, nil
}

func (p *OpenAIProvider) Embed(ctx context.Context, input string) (providers.EmbeddingResult, error) {
	embedReq := openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(p.embedModel),
		Input: input,
	}

	response, err := p.client.CreateEmbeddings(ctx, embedReq)
	if err != nil {
		return providers.EmbeddingResult{}, err
	}

	if len(response.Data) == 0 {
		return providers.EmbeddingResult{}, nil
	}

	embedding := response.Data[0].Embedding

	return providers.EmbeddingResult{
		Vector:     embedding,
		Dimensions: len(embedding),
		Model:      p.embedModel,
	}, nil
}

func (p *OpenAIProvider) buildMessages(systemPrompt, userPrompt string) []openai.ChatCompletionMessage {
	messages := []openai.ChatCompletionMessage{}

	if systemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	return messages
}
