package providers

import "context"

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float32
}

type CompletionResponse struct {
	Content   string
	TokenUsed int
	Model     string
	Provider  string
}

type EmbeddingResult struct {
	Vector     []float32
	Dimensions int
	Model      string
}

type LLMProvider interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Embed(ctx context.Context, input string) (EmbeddingResult, error)
	Name() string
	MaxTokens() int
}

// func Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
// 	return CompletionResponse{}, nil
// }
// func Embed(ctx context.Context, input string) (EmbeddingResult, error) {
// 	return EmbeddingResult{}, nil
// }
// func Name() string {
// 	return "base"
// }

// func MaxTokens() int {
// 	return 0
// }
