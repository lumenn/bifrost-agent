package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TestData struct {
	Question string `json:"question"`
	Answer   int    `json:"answer"`
	Test     *struct {
		Q string `json:"q"`
		A string `json:"a"`
	} `json:"test,omitempty"`
}

type CentralaData struct {
	APIKey      string     `json:"apikey"`
	Description string     `json:"description"`
	Copyright   string     `json:"copyright"`
	TestData    []TestData `json:"test-data"`
}

func ProcessCentralaData(baseURL, apiKey string, llmService LLMService) (*CentralaData, error) {
	// Construct the full URL
	url := fmt.Sprintf("%s/data/%s/json.txt", baseURL, apiKey)
	llmService.SetSystemPrompt(`
	You are a helpful assistant that corrects the answers to multiple questions in the test.
	You are given multiple questions.
	Return answers in JSON array format, where each element contains question and answer.

	Example:
	Questions: 
	1. What is the capital of France?
	2. What is the capital of Germany?
	
	Response: [
		{"question": "What is the capital of France?", "answer": "Paris"},
		{"question": "What is the capital of Germany?", "answer": "Berlin"}
	]
	`)

	// Download the JSON data
	jsonData, err := GetRequestBody(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download data: %w", err)
	}

	// Parse the JSON data
	var data CentralaData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	correctedTestData := make([]TestData, len(data.TestData))
	copy(correctedTestData, data.TestData)

	// Collect all test questions
	var testQuestions []string
	testIndices := make(map[int]bool) // Keep track of indices with test questions
	for i, td := range data.TestData {
		if td.Test != nil {
			testQuestions = append(testQuestions, td.Test.Q)
			testIndices[i] = true
		}
	}

	// If we have test questions, process them in batch
	if len(testQuestions) > 0 {
		// Create a single string with all questions
		batchQuestion := "Please answer these questions:\n"
		for i, q := range testQuestions {
			batchQuestion += fmt.Sprintf("%d. %s\n", i+1, q)
		}

		// Get answers for all questions at once
		llmResponse, err := llmService.SendChatMessage(batchQuestion)
		if err != nil {
			return nil, fmt.Errorf("failed to get LLM response: %w", err)
		}

		var llmAnswers []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		}
		if err := json.Unmarshal([]byte(llmResponse), &llmAnswers); err != nil {
			return nil, fmt.Errorf("failed to parse LLM response '%s': %w", llmResponse, err)
		}

		// Update answers in correctedTestData
		answerIndex := 0
		for i := range correctedTestData {
			if testIndices[i] {
				if answerIndex < len(llmAnswers) {
					correctedTestData[i].Test.A = llmAnswers[answerIndex].Answer
					answerIndex++
				}
			}
		}
	}

	// Process mathematical questions
	for i, td := range data.TestData {
		if !testIndices[i] && strings.Contains(td.Question, "+") {
			parts := strings.Split(td.Question, "+")
			if len(parts) != 2 {
				continue
			}

			var num1, num2 int
			_, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &num1)
			if err != nil {
				continue
			}
			_, err = fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &num2)
			if err != nil {
				continue
			}

			sum := num1 + num2
			if sum != td.Answer {
				correctedTestData[i].Answer = sum
			}
		}
	}

	response := &CentralaData{
		APIKey:      apiKey,
		Description: data.Description,
		Copyright:   data.Copyright,
		TestData:    correctedTestData,
	}

	return response, nil
}

func GetCensorshipData(baseURL, apiKey string) (string, error) {
	// Construct the URL using the provided base URL and API key
	url := fmt.Sprintf("%s/data/%s/cenzura.txt", baseURL, apiKey)

	// Use the existing GetRequestBody function to fetch the content
	content, err := GetRequestBody(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch censorship data: %w", err)
	}

	return content, nil
}
