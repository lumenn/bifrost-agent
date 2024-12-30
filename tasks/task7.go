package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

type AnalysisReport struct {
	People   []string `json:"people"`
	Hardware []string `json:"hardware"`
}

func SolveTask7(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	openAIService, ok := llmService.(*services.OpenAiService)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "LLM service is not an OpenAI service",
		})
		return
	}

	report, response, err := processTask7(centralaBaseURL, centralaAPIKey, openAIService)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to process task: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"sent":     report,
		"response": response,
	})
}

func processTask7(baseURL, apiKey string, openAI *services.OpenAiService) (*AnalysisReport, map[string]interface{}, error) {
	workDir := filepath.Join("/tmp", "task7")
	openAI.SetSystemPrompt("follow speciified instrictuions with much care")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Download and extract files
	zipPath := filepath.Join(workDir, "files.zip")
	downloadURL := fmt.Sprintf("%s/dane/pliki_z_fabryki.zip", baseURL)

	if !services.FileExists(zipPath) {
		if err := services.DownloadFile(downloadURL, zipPath); err != nil {
			return nil, nil, fmt.Errorf("failed to download files: %w", err)
		}
	}

	if err := services.UnzipFile(zipPath, workDir, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to extract files: %w", err)
	}

	filePaths, err := services.ListFiles(workDir, ".zip")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list files: %w", err)
	}

	if len(filePaths) == 0 {
		return nil, nil, fmt.Errorf("no files found to process")
	}

	// Initialize with non-nil slices and capacity
	report := &AnalysisReport{
		People:   make([]string, 0, len(filePaths)),
		Hardware: make([]string, 0, len(filePaths)),
	}

	processedFiles := 0
	for _, filePath := range filePaths {
		category, err := analyzeFile(filePath, openAI)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to analyze %s: %w", filepath.Base(filePath), err)
		}

		fileName := filepath.Base(filePath)
		switch category {
		case "people":
			report.People = append(report.People, fileName)
			processedFiles++
		case "hardware":
			report.Hardware = append(report.Hardware, fileName)
			processedFiles++
		}
	}

	if processedFiles == 0 {
		return nil, nil, fmt.Errorf("no files were successfully categorized")
	}

	// Verify report before submission
	if len(report.People) == 0 && len(report.Hardware) == 0 {
		return nil, nil, fmt.Errorf("both categories are empty after processing")
	}

	response, err := submitReport(report, baseURL, apiKey)
	if err != nil {
		return nil, nil, err
	}

	return report, response, nil
}

func analyzeFile(filePath string, openAI *services.OpenAiService) (string, error) {
	var content string
	var err error

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".txt", ".md":
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			return "", err
		}
		content = string(contentBytes)
	case ".mp3", ".wav", ".m4a":
		content, err = openAI.TranscribeAudio(filePath)
		if err != nil {
			return "", err
		}
	case ".jpg", ".jpeg", ".png":
		content, err = openAI.AnalyzeImages(filePath)
		if err != nil {
			return "", err
		}
	default:
		return "other", nil
	}

	openAI.SetSystemPrompt(`Classify future content into exactly one category. Respond with only one word:
	- "people" if the content is about captured people or about signs of their location.
		IF REPORT IS ABOUT NO PEOPLE SIGNS CATEGORIZE IT AS OTHER
	- "hardware" if the content is about equipment or devices being fixed
	- "other" for anything else (for example software fixes)`)

	response, err := openAI.SendChatMessage(content)
	if err != nil {
		return "", err
	}

	cleanResponse := strings.ToLower(strings.TrimSpace(response))

	// Ensure exact category matching
	switch cleanResponse {
	case "people", "hardware":
		return cleanResponse, nil
	default:
		return "other", nil
	}
}

func submitReport(report *AnalysisReport, baseURL, apiKey string) (map[string]interface{}, error) {
	reportRequest := map[string]interface{}{
		"task":   "kategorie",
		"apikey": apiKey,
		"answer": report,
	}

	reportURL := fmt.Sprintf("%s/report", baseURL)
	responseStr, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		return nil, err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal([]byte(responseStr), &responseMap); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return responseMap, nil
}
