package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func (s OpenAiService) TranscribeAudio(audioPath string) (string, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: audioPath,
	}

	resp, err := s.client.CreateTranscription(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to transcribe audio: %w", err)
	}

	return resp.Text, nil
}

func (s OpenAiService) readTranscriptionFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read transcription file: %w", err)
	}
	return string(content), nil
}

func (s OpenAiService) writeTranscriptionFile(audioPath string, transcription string) error {
	// Create markdown file path by replacing audio extension with .md
	mdPath := audioPath[:len(audioPath)-len(filepath.Ext(audioPath))] + ".md"

	err := os.WriteFile(mdPath, []byte(transcription), 0644)
	if err != nil {
		return fmt.Errorf("failed to write transcription file: %w", err)
	}
	return nil
}

func (s OpenAiService) getTranscription(audioPath string) (string, error) {
	// Check for existing markdown transcription
	mdPath := audioPath[:len(audioPath)-len(filepath.Ext(audioPath))] + ".md"

	// Try to read existing transcription
	if transcription, err := s.readTranscriptionFile(mdPath); err == nil {
		return transcription, nil
	}

	// If no transcription exists, create new one
	transcription, err := s.TranscribeAudio(audioPath)
	if err != nil {
		return "", err
	}

	// Save the new transcription
	if err := s.writeTranscriptionFile(audioPath, transcription); err != nil {
		return "", err
	}

	return transcription, nil
}

func (s OpenAiService) TranscribeDirectory(dirPath string) (map[string]string, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	transcriptions := make(map[string]string)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if file is an audio file
		ext := filepath.Ext(file.Name())
		if ext != ".mp3" && ext != ".wav" && ext != ".m4a" {
			continue
		}

		fullPath := filepath.Join(dirPath, file.Name())
		transcription, err := s.getTranscription(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get transcription for %s: %w", file.Name(), err)
		}

		transcriptions[file.Name()] = transcription
	}

	return transcriptions, nil
}
