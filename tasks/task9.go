package tasks

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask9(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	log.Println("[INFO] Starting Task9 execution")

	llmService.SetSystemPrompt(
		`You have two tasks: 1. Analyse text and return keywords, 2. Help choosing the context files. 
		<rules>
		<rule>Always respond in format asked for, and nothing else.</rule>
		<rule>Never use markdown code blocks</rule>
		<rule>'''json is disallowed</rule>
		</rules>
		`)

	workDir := "/tmp/task9"
	if err := os.MkdirAll(workDir, os.ModePerm); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create work directory: %v", err)})
		return
	}

	// Download and extract files
	zipPath := filepath.Join(workDir, "files.zip")
	downloadURL := fmt.Sprintf("%s/dane/pliki_z_fabryki.zip", centralaBaseURL)

	if !services.FileExists(zipPath) {
		if err := services.DownloadFile(downloadURL, zipPath); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to download files: %v", err)})
			return
		}
	}

	if err := services.UnzipFile(zipPath, workDir, nil); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to extract files: %v", err)})
		return
	}

	files, err := services.ListFiles(workDir, ".zip")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list files: %v", err)})
		return
	}

	// Filter files to include only .txt files
	var txtFiles []string
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file), ".txt") {
			txtFiles = append(txtFiles, file)
		}
	}

	// Collect facts from /facts directory
	factsDirectory := filepath.Join(workDir, "facts")
	factsContent := make(map[string]string)
	factsFiles, err := os.ReadDir(factsDirectory)
	if err != nil {
		log.Printf("[ERROR] Failed to read facts directory: %v", err)
	} else {
		for _, file := range factsFiles {
			if file.IsDir() {
				continue
			}
			content, err := os.ReadFile(filepath.Join(factsDirectory, file.Name()))
			if err != nil {
				log.Printf("[ERROR] Failed to read fact file %s: %v", file.Name(), err)
				continue
			}
			factsContent[file.Name()] = string(content)
		}
	}

	// Add both facts and reports analysis caching
	factsAnalysisPath := filepath.Join(workDir, "facts_analysis.json")
	reportsAnalysisPath := filepath.Join(workDir, "reports_analysis.json")
	factsAnalysis := make(map[string]string)
	reportsAnalysis := make(map[string]string)

	// Try to load cached analyses
	if cachedData, err := os.ReadFile(factsAnalysisPath); err == nil {
		if err := json.Unmarshal(cachedData, &factsAnalysis); err != nil {
			log.Printf("[WARN] Failed to parse cached facts analysis: %v", err)
		}
	}

	if cachedData, err := os.ReadFile(reportsAnalysisPath); err == nil {
		if err := json.Unmarshal(cachedData, &reportsAnalysis); err != nil {
			log.Printf("[WARN] Failed to parse cached reports analysis: %v", err)
		}
	}

	// If cache is empty, perform facts analysis
	if len(factsAnalysis) == 0 {
		log.Println("[INFO] Analyzing facts files")
		for fileName, content := range factsContent {
			analysisPrompt := fmt.Sprintf(`Please provide a short description of the following fact:
                Content: %s
                Return only the description, nothing else. Use polish language. Important informations are: sectors, locations, people, job titles`, content)

			description, err := llmService.SendChatMessage(analysisPrompt)
			if err != nil {
				log.Printf("[ERROR] Facts analysis failed for %s: %v", fileName, err)
				continue
			}
			factsAnalysis[fileName] = strings.TrimSpace(description)
		}

		// Cache the facts analysis
		if cachedData, err := json.Marshal(factsAnalysis); err == nil {
			if err := os.WriteFile(factsAnalysisPath, cachedData, 0644); err != nil {
				log.Printf("[WARN] Failed to cache facts analysis: %v", err)
			}
		}
	}

	// Analyze reports if not cached
	if len(reportsAnalysis) == 0 {
		log.Println("[INFO] Analyzing report files")
		for _, file := range txtFiles {
			if filepath.Dir(file) == factsDirectory {
				continue
			}

			fileName := filepath.Base(file)
			// Reports are in the root directory
			reportPath := filepath.Join(workDir, fileName)

			content, err := os.ReadFile(reportPath)
			if err != nil {
				log.Printf("[ERROR] Failed to read file %s: %v", fileName, err)
				continue
			}

			analysisPrompt := fmt.Sprintf(`Please provide a short description of the following report:
                Content: %s
                Return only the description, nothing else. Use polish language. Max 2 sentences. Important informations are: sectors, locations, people, job titles`, string(content))

			description, err := llmService.SendChatMessage(analysisPrompt)
			if err != nil {
				log.Printf("[ERROR] Report analysis failed for %s: %v", fileName, err)
				continue
			}
			reportsAnalysis[fileName] = strings.TrimSpace(description)
		}

		// Cache the reports analysis
		if cachedData, err := json.Marshal(reportsAnalysis); err == nil {
			if err := os.WriteFile(reportsAnalysisPath, cachedData, 0644); err != nil {
				log.Printf("[WARN] Failed to cache reports analysis: %v", err)
			}
		}
	}

	// Modify the context prompt to include both facts and reports descriptions
	fileAnalysis := make(map[string]string)
	for _, file := range txtFiles {
		if filepath.Dir(file) == factsDirectory {
			continue
		}

		fileName := filepath.Base(file)
		log.Printf("[INFO] Analyzing file: %s", fileName)

		// Read from root directory for reports
		content, err := os.ReadFile(filepath.Join(workDir, fileName))
		if err != nil {
			log.Printf("[ERROR] Failed to read file %s: %v", fileName, err)
			continue
		}

		// Modified context prompt to include both facts and reports
		contextPrompt := fmt.Sprintf(`I have a file named "%s". Here's what I know about it:
		%s

		I have access to the following additional materials:

		Facts:
		%s

		Related Reports:
		%s

		Should I include any of these materials to better understand the content? 
		Prepare response as json object with both facts and reports: 
		{ 
			"facts": { "filename01.ext": 1, "filename0X.ext": 0 },
			"reports": { "2024-11-12_report-XX-sektor_XX.ext": 1, "2024-11-12_report-XX-sektor_XX.ext": 0 }
		}
		Mark all files as either 1 (needed) or 0 (not needed). No additional formatting.`,
			fileName,
			string(content),
			func() string {
				var descriptions []string
				for fname, desc := range factsAnalysis {
					descriptions = append(descriptions, fmt.Sprintf("%s: %s", fname, desc))
				}
				return strings.Join(descriptions, "\n")
			}(),
			func() string {
				var descriptions []string
				for fname, desc := range reportsAnalysis {
					if fname != fileName { // Exclude current file
						descriptions = append(descriptions, fmt.Sprintf("%s: %s", fname, desc))
					}
				}
				return strings.Join(descriptions, "\n")
			}())

		needsContext, err := llmService.SendChatMessage(contextPrompt)
		if err != nil {
			log.Printf("[ERROR] Context check failed for %s: %v", fileName, err)
			continue
		}

		// Parse modified context response
		var contextNeeded struct {
			Facts   map[string]int `json:"facts"`
			Reports map[string]int `json:"reports"`
		}
		if err := json.Unmarshal([]byte(needsContext), &contextNeeded); err != nil {
			log.Printf("[ERROR] Failed to parse context response for %s: %v", fileName, err)
			continue
		}

		// Build analysis prompt with relevant context from both facts and reports
		var contextBuilder strings.Builder
		contextBuilder.WriteString(fmt.Sprintf("Content of file %s:\n%s\n\n", fileName, string(content)))

		// Add relevant facts with proper path handling
		for factFile, needed := range contextNeeded.Facts {
			if needed == 1 {
				factContent, err := os.ReadFile(filepath.Join(workDir, "facts", factFile))
				if err != nil {
					log.Printf("[ERROR] Failed to read fact file %s: %v", factFile, err)
					continue
				}
				contextBuilder.WriteString(fmt.Sprintf("Additional fact from %s:\n%s\n\n",
					factFile, string(factContent)))
			}
		}

		for reportFile, needed := range contextNeeded.Reports {
			if needed == 1 {
				// Read reports from root directory
				reportContent, err := os.ReadFile(filepath.Join(workDir, reportFile))
				if err != nil {
					log.Printf("[WARN] Failed to read report file %s: %v", reportFile, err)
					continue
				}
				contextBuilder.WriteString(fmt.Sprintf("Additional report from %s:\n%s\n\n",
					reportFile, string(reportContent)))
			}
		}

		analysisPrompt := contextBuilder.String() + `
		For response use Polish language. 
		Based on the above content, please provide keywords describing the main topics. 
		Return only the keywords, separated by commas. NOTHING ELSE. mportant informations are: sectors, locations, people, job titles.
		Does the report mention arrest capture (zatrzymanie) or control of a person? if so use that as a keyword.
		Does the report mention animals? if so use that as a keyword.
		Does the report mention programming language? if so use that as a keyword.
		Sectors should be always fully qualified like A1 B2, never A or B.
		`

		keywords, err := llmService.SendChatMessage(analysisPrompt)
		if err != nil {
			log.Printf("[ERROR] Analysis failed for %s: %v", fileName, err)
			continue
		}

		fileAnalysis[fileName] = strings.TrimSpace(keywords)
	}

	// Send response to task endpoint
	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)

	reportRequest := map[string]interface{}{
		"task":   "dokumenty",
		"apikey": centralaAPIKey,
		"answer": fileAnalysis,
	}

	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send response: %v", err)})
		return
	}

	log.Println("[INFO] Task9 completed successfully")
	ctx.JSON(http.StatusOK, gin.H{
		"reportRequest": reportRequest,
		"response":      response,
	})
}
