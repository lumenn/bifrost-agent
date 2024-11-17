package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type OllamaService struct {
	client *api.Client
	model  string
	prompt string
}

func NewOllamaService(baseURL string, systemPrompt string, model string) (*OllamaService, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL not specified")
	}

	if model == "" {
		return nil, fmt.Errorf("model not specified")
	}

	// Parse the base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create the Ollama client - note that NewClient only returns the client
	client := api.NewClient(parsedURL, httpClient)

	return &OllamaService{
		client: client,
		model:  model,
		prompt: systemPrompt,
	}, nil
}

func (s *OllamaService) SendChatMessage(message string) (string, error) {
	if message == "" {
		return "", fmt.Errorf("empty message provided")
	}

	messages := []api.Message{
		{
			Role:    "system",
			Content: s.prompt,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Process this text and replace all sensitive information: %s", message),
		},
	}

	// Create the chat request
	request := api.ChatRequest{
		Model:    s.model,
		Messages: messages,
		Stream:   nil,
	}

	var fullResponse strings.Builder

	// Define response handler function
	responseHandler := func(r api.ChatResponse) error {
		if r.Message.Content != "" {
			fullResponse.WriteString(r.Message.Content)
		}
		return nil
	}

	// Make the API call with response handler
	err := s.client.Chat(context.Background(), &request, responseHandler)
	if err != nil {
		return "", fmt.Errorf("chat request failed: %w", err)
	}

	response := fullResponse.String()
	if response == "" {
		return "", fmt.Errorf("received empty response from Ollama")
	}

	return strings.TrimSpace(response), nil
}

func (s *OllamaService) SetSystemPrompt(prompt string) {
	s.prompt = prompt
}
