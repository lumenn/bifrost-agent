package tasks

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask10(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	log.Println("[INFO] Starting Task10 execution")

	workDir := "/tmp/task10"
	if err := os.MkdirAll(workDir, os.ModePerm); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create work directory: %v", err)})
		return
	}

	// Download and extract initial zip file
	zipPath := filepath.Join(workDir, "files.zip")
	downloadURL := fmt.Sprintf("%s/dane/pliki_z_fabryki.zip", centralaBaseURL)

	if !services.FileExists(zipPath) {
		if err := services.DownloadFile(downloadURL, zipPath); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to download files: %v", err)})
			return
		}
	}

	// Extract the initial zip file
	if err := services.UnzipFile(zipPath, workDir, nil); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to extract files: %v", err)})
		return
	}

	// Extract the weapons_tests.zip with password
	weaponsZipPath := filepath.Join(workDir, "weapons_tests.zip")
	weaponsDir := filepath.Join(workDir, "weapons")
	if err := services.UnzipFile(weaponsZipPath, weaponsDir, &[]string{"1670"}[0]); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to extract weapons tests: %v", err)})
		return
	}

	// Initialize vector database service
	vectorDB, err := services.NewQdrantService()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize vector database: %v", err)})
		return
	}

	// Index all reports from weapons directory
	files, err := services.ListFiles(filepath.Join(weaponsDir, "do-not-share"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list files: %v", err)})
		return
	}

	// Process and index each file
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			log.Printf("[ERROR] Failed to read file %s: %v", file, err)
			continue
		}

		// Extract date from filename or content
		date, err := extractDateFromContent(string(file))
		date = strings.ReplaceAll(date, "_", "-")
		if err != nil {
			log.Printf("[ERROR] Failed to extract date from %s: %v", file, err)
			continue
		}

		// Index the document with metadata
		metadata := map[string]interface{}{
			"date":     date,
			"filename": filepath.Base(file),
			"path":     file,
		}

		if err := vectorDB.IndexDocument(string(content), metadata); err != nil {
			log.Printf("[ERROR] Failed to index document %s: %v", file, err)
			continue
		}
	}

	// Search for weapon prototype theft
	query := "W raporcie, z którego dnia znajduje się wzmianka o kradzieży prototypu broni?"
	results, err := vectorDB.Search(query, 1)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to search vector database: %v", err)})
		return
	}

	if len(results) == 0 {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "No matching documents found"})
		return
	}

	// Extract date from top result
	value := results[0].GetPayload()
	metadata := value["metadata"]
	date := metadata.GetStructValue().GetFields()["date"].GetStringValue()
	content := value["content"]

	log.Println(metadata, content)

	// Send report to centrala
	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
	reportRequest := map[string]interface{}{
		"task":   "wektory",
		"apikey": centralaAPIKey,
		"answer": date,
	}

	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send report: %v", err)})
		return
	}

	log.Println("[INFO] Task10 completed successfully")
	ctx.JSON(http.StatusOK, gin.H{
		"reportRequest": reportRequest,
		"response":      response,
	})
}

func extractDateFromContent(content string) (string, error) {
	// Try to find date in YYYY-MM-DD format
	dateRegex := `\d{4}_\d{2}_\d{2}`
	matches := regexp.MustCompile(dateRegex).FindStringSubmatch(content)
	if len(matches) > 0 {
		return matches[0], nil
	}

	// If no date found, try parsing the content with LLM to extract the date
	// This would be implemented if needed
	return "", fmt.Errorf("no date found in content")
}
