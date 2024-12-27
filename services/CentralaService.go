package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
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

type MediaInfo struct {
	Type        string // "audio" or "image"
	URL         string
	Description string
}

type CentralaService struct {
	baseURL       string
	apiKey        string
	openAIService *OpenAiService
}

type APIResponse struct {
	Reply interface{} `json:"reply"`
	Error string      `json:"error"`
}

type EntityResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type DatabaseResponse struct {
	Reply interface{} `json:"reply"`
	Error string      `json:"error"`
}

type TableInfo struct {
	TableName string `json:"Tables_in_banan"`
}

type TableStructure struct {
	Table       string `json:"Table"`
	CreateTable string `json:"Create Table"`
}

type DatabaseRequest struct {
	Task   string `json:"task"`
	APIKey string `json:"apikey"`
	Query  string `json:"query"`
}

func NewCentralaService(baseURL, apiKey string, openAIService *OpenAiService) *CentralaService {
	return &CentralaService{
		baseURL:       baseURL,
		apiKey:        apiKey,
		openAIService: openAIService,
	}
}

func (s *CentralaService) ProcessCentralaData(llmService LLMService) (*CentralaData, error) {
	// Construct the full URL
	url := fmt.Sprintf("%s/data/%s/json.txt", s.baseURL, s.apiKey)
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
		APIKey:      s.apiKey,
		Description: data.Description,
		Copyright:   data.Copyright,
		TestData:    correctedTestData,
	}

	return response, nil
}

func (s *CentralaService) GetCensorshipData() (string, error) {
	// Construct the URL using the provided base URL and API key
	url := fmt.Sprintf("%s/data/%s/cenzura.txt", s.baseURL, s.apiKey)

	// Use the existing GetRequestBody function to fetch the content
	content, err := GetRequestBody(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch censorship data: %w", err)
	}

	return content, nil
}

func (s *CentralaService) GetArxivQuestions() (map[string]string, error) {
	url := fmt.Sprintf("%s/data/%s/arxiv.txt", s.baseURL, s.apiKey)
	content, err := GetRequestBody(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch questions: %w", err)
	}

	questions := make(map[string]string)
	err = json.Unmarshal([]byte(content), &questions)
	if err != nil {
		return nil, fmt.Errorf("failed to parse questions: %w", err)
	}

	return questions, nil
}

func (s *CentralaService) ProcessArxivPage(url string) (string, []MediaInfo, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var mediaInfos []MediaInfo
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Process audio files
	doc.Find("audio source").Each(func(i int, sel *goquery.Selection) {
		if src, exists := sel.Attr("src"); exists {
			wg.Add(1)
			go func(audioURL string) {
				defer wg.Done()
				if transcription, err := s.openAIService.TranscribeAudio(audioURL); err == nil {
					mu.Lock()
					mediaInfos = append(mediaInfos, MediaInfo{
						Type:        "audio",
						URL:         audioURL,
						Description: transcription,
					})
					mu.Unlock()
				}
			}(src)
		}
	})

	// Process images
	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		if src, exists := sel.Attr("src"); exists {
			wg.Add(1)
			go func(imgURL string) {
				defer wg.Done()
				if description, err := s.openAIService.AnalyzeImage(imgURL); err == nil {
					mu.Lock()
					mediaInfos = append(mediaInfos, MediaInfo{
						Type:        "image",
						URL:         imgURL,
						Description: description,
					})
					mu.Unlock()
				}
			}(src)
		}
	})

	wg.Wait()

	// Create enhanced HTML with media descriptions
	doc.Find("audio").Each(func(i int, sel *goquery.Selection) {
		source := sel.Find("source")
		if src, exists := source.Attr("src"); exists {
			for _, info := range mediaInfos {
				if info.URL == src {
					sel.AfterHtml(fmt.Sprintf("<p class='media-description'>Audio transcription: %s</p>", info.Description))
					break
				}
			}
		}
	})

	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		if src, exists := sel.Attr("src"); exists {
			for _, info := range mediaInfos {
				if info.URL == src {
					sel.AfterHtml(fmt.Sprintf("<p class='media-description'>Image description: %s</p>", info.Description))
					break
				}
			}
		}
	})

	enhancedHTML, err := doc.Html()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate enhanced HTML: %w", err)
	}

	return enhancedHTML, mediaInfos, nil
}

func (s *CentralaService) AnswerArxivQuestions(questions map[string]string, context string) (map[string]string, error) {
	answers := make(map[string]string)

	contextPrompt := fmt.Sprintf(`Based on the following context, answer the question in a single, concise sentence.

Context:
%s

`, context)

	for id, question := range questions {
		prompt := contextPrompt + fmt.Sprintf("Question: %s\nProvide a single sentence answer:", question)

		answer, err := s.openAIService.SendChatMessage(prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to get answer for question %s: %w", id, err)
		}

		answer = strings.TrimSpace(answer)
		answer = strings.Trim(answer, `"'`)

		answers[id] = answer
	}

	return answers, nil
}

func (s *CentralaService) QueryAPI(endpoint, query string) (interface{}, error) {
	request := map[string]interface{}{
		"apikey": s.apiKey,
		"query":  query,
	}

	response, err := PostJSON(fmt.Sprintf("%s%s", s.baseURL, endpoint), request)
	if err != nil {
		return nil, fmt.Errorf("failed to query API: %w", err)
	}

	var apiResponse EntityResponse
	if err := json.Unmarshal([]byte(response), &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if apiResponse.Code != 0 {
		return nil, fmt.Errorf("API error: code %d", apiResponse.Code)
	}

	return apiResponse, nil
}

func (s *CentralaService) QueryDatabase(query string) (*DatabaseResponse, error) {
	request := DatabaseRequest{
		Task:   "database",
		APIKey: s.apiKey,
		Query:  query,
	}

	response, err := PostJSON(fmt.Sprintf("%s/apidb", s.baseURL), request)
	if err != nil {
		return nil, fmt.Errorf("failed to query database: %w", err)
	}

	var dbResponse DatabaseResponse
	if err := json.Unmarshal([]byte(response), &dbResponse); err != nil {
		return nil, fmt.Errorf("failed to parse database response: %w", err)
	}

	if dbResponse.Error != "OK" {
		return nil, fmt.Errorf("database error: %s", dbResponse.Error)
	}

	return &dbResponse, nil
}

func (s *CentralaService) ShowTables() ([]string, error) {
	response, err := s.QueryDatabase("SHOW TABLES")
	if err != nil {
		return nil, err
	}

	// Convert the reply to JSON to parse it
	replyJSON, err := json.Marshal(response.Reply)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reply: %w", err)
	}

	var tables []TableInfo
	if err := json.Unmarshal(replyJSON, &tables); err != nil {
		return nil, fmt.Errorf("failed to parse tables: %w", err)
	}

	// Extract table names
	tableNames := make([]string, len(tables))
	for i, table := range tables {
		tableNames[i] = table.TableName
	}

	return tableNames, nil
}

func (s *CentralaService) ShowCreateTable(tableName string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE %s", tableName)
	response, err := s.QueryDatabase(query)
	if err != nil {
		return "", err
	}

	// Convert the reply to JSON to parse it
	replyJSON, err := json.Marshal(response.Reply)
	if err != nil {
		return "", fmt.Errorf("failed to marshal reply: %w", err)
	}

	var structures []TableStructure
	if err := json.Unmarshal(replyJSON, &structures); err != nil {
		return "", fmt.Errorf("failed to parse table structure: %w", err)
	}

	if len(structures) == 0 {
		return "", fmt.Errorf("no structure returned for table %s", tableName)
	}

	return structures[0].CreateTable, nil
}
