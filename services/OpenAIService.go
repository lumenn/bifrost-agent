package services

import (
	"context"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	defaultMaxTokens = 1000
	defaultTimeout   = 30 * time.Second
	defaultModel     = openai.GPT4
)

type LLMService interface {
	SendChatMessage(string) (string, error)
	SetSystemPrompt(string)
}

type OpenAiService struct {
	ApiKey       string
	SystemPrompt string
	Model        string
	client       *openai.Client
}

func NewOpenAIService(apiKey string, systemPrompt string, model string) (*OpenAiService, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not specified - make sure to set environment variable")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt not specified - make sure to set it")
	}

	if model == "" {
		model = defaultModel
	}

	client := openai.NewClient(apiKey)
	return &OpenAiService{
		ApiKey:       apiKey,
		SystemPrompt: systemPrompt,
		Model:        model,
		client:       client,
	}, nil
}

func (s OpenAiService) SendChatMessage(message string) (string, error) {
	if message == "" {
		return "", fmt.Errorf("empty message provided")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model: s.Model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: s.SystemPrompt,
			},
			{
				Role:    "user",
				Content: message,
			},
		},
		MaxTokens: defaultMaxTokens,
	}

	openaiResp, err := s.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from API")
	}

	return openaiResp.Choices[0].Message.Content, nil
}

func (s *OpenAiService) SetSystemPrompt(prompt string) {
	s.SystemPrompt = prompt
}
