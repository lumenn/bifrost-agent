package tasks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"log"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func downloadFile(baseURL, url string) (string, error) {
	fullURL := fmt.Sprintf("%s/%s/%s", baseURL, "dane", url)
	log.Printf("[DEBUG] Downloading file from URL: %s", fullURL)
	resp, err := http.Get(fullURL)
	if err != nil {
		log.Printf("[ERROR] Download failed from URL %s: %v", fullURL, err)
		return "", err
	}
	defer resp.Body.Close()

	// Extract file extension from URL
	ext := filepath.Ext(url)
	if ext == "" {
		ext = ".tmp"
	}

	// Create temp file with proper extension
	tempFile, err := os.CreateTemp("", "media-*"+ext)
	if err != nil {
		log.Printf("[ERROR] Failed to create temp file for %s: %v", fullURL, err)
		return "", err
	}
	defer tempFile.Close()

	bytesWritten, err := io.Copy(tempFile, resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to write file content from %s: %v", fullURL, err)
		return "", err
	}

	log.Printf("[INFO] Successfully downloaded %d bytes from %s to %s", bytesWritten, fullURL, tempFile.Name())
	return tempFile.Name(), nil
}

func getSHA256Hash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getCachedDescription(hash string) (string, error) {
	cacheFilePath := filepath.Join("/tmp/task8", hash+".txt")
	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func cacheDescription(hash, description string) error {
	cacheDir := "/tmp/task8"
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return err
	}

	cacheFilePath := filepath.Join(cacheDir, hash+".txt")
	return os.WriteFile(cacheFilePath, []byte(description), os.ModePerm)
}

func extractTextContent(doc *goquery.Document) string {
	var content strings.Builder

	// Extract regular text
	doc.Find("body").Each(func(i int, s *goquery.Selection) {
		s.Find("*").Each(func(i int, s *goquery.Selection) {
			if goquery.NodeName(s) != "script" && goquery.NodeName(s) != "style" {
				text := strings.TrimSpace(s.Text())
				if text != "" {
					content.WriteString(text + "\n")
				}
			}
		})
	})

	// Extract media descriptions
	doc.Find(".media-description").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			content.WriteString(text + "\n")
		}
	})

	return content.String()
}

func SolveTask8(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	log.Println("[INFO] Starting Task8 execution")

	openAIService, ok := llmService.(*services.OpenAiService)
	if !ok {
		log.Println("[ERROR] LLM service type assertion failed - expected OpenAI service")
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "LLM service is not an OpenAI service",
		})
		return
	}

	openAIService.SetSystemPrompt(`
		You are a text processing assistant. 
		Your task is to analyze the provided HTML document and answer asked questions. 
		Use one short sentences.

		return answers 1 line per answer
		Answer to question 1
		Answer to question 2
		Answer to question 3
	`)

	// Fetch HTML data
	arxivHTMLURL := fmt.Sprintf("%s/dane/arxiv-draft.html", centralaBaseURL)
	log.Printf("[DEBUG] Fetching HTML content from URL: %s", arxivHTMLURL)
	resp, err := http.Get(arxivHTMLURL)
	if err != nil {
		log.Printf("[ERROR] HTML fetch failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch arxiv HTML: %v", err)})
		return
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to parse arxiv HTML: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse arxiv HTML: %v", err)})
		return
	}

	var mediaInfos []services.MediaInfo
	doc.Find("audio source").Each(func(i int, sel *goquery.Selection) {
		if src, exists := sel.Attr("src"); exists {
			log.Printf("[DEBUG] Processing audio file: %s", src)
			localPath, err := downloadFile(centralaBaseURL, src)
			if err != nil {
				log.Printf("[ERROR] Audio download failed for %s: %v", src, err)
				return
			}

			hash, err := getSHA256Hash(localPath)
			if err != nil {
				log.Printf("[ERROR] Hash computation failed for audio %s: %v", src, err)
				return
			}

			log.Printf("[DEBUG] Checking cache for audio hash: %s", hash)
			description, err := getCachedDescription(hash)
			if err != nil {
				log.Printf("[ERROR] Cache retrieval failed for audio %s: %v", src, err)
				return
			}

			if description == "" {
				log.Printf("[INFO] Cache miss - transcribing audio: %s", localPath)
				description, err = openAIService.TranscribeAudio(localPath)
				if err != nil {
					log.Printf("[ERROR] Audio transcription failed for %s: %v", src, err)
					return
				}
				if err := cacheDescription(hash, description); err != nil {
					log.Printf("[ERROR] Cache storage failed for audio %s: %v", src, err)
				}
			} else {
				log.Printf("[DEBUG] Using cached audio description for: %s", src)
			}

			mediaInfos = append(mediaInfos, services.MediaInfo{
				Type:        "audio",
				URL:         src,
				Description: description,
			})
		}
	})

	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		if src, exists := sel.Attr("src"); exists {
			log.Printf("Downloading image from %s", src)
			localPath, err := downloadFile(centralaBaseURL, src)
			if err != nil {
				log.Printf("[ERROR] Failed to download image from %s: %v", src, err)
				return
			}

			hash, err := getSHA256Hash(localPath)
			if err != nil {
				log.Printf("[ERROR] Failed to compute hash for image %s: %v", src, err)
				return
			}

			description, err := getCachedDescription(hash)
			if err != nil {
				log.Printf("[ERROR] Failed to get cached description for image %s: %v", src, err)
				return
			}

			if description == "" {
				log.Printf("Analyzing image from %s", localPath)
				description, err = openAIService.AnalyzeImage(localPath)
				if err != nil {
					log.Printf("[ERROR] Failed to analyze image from %s: %v", src, err)
					return
				}
				if err := cacheDescription(hash, description); err != nil {
					log.Printf("[ERROR] Failed to cache description for image %s: %v", src, err)
				}
			}

			mediaInfos = append(mediaInfos, services.MediaInfo{
				Type:        "image",
				URL:         src,
				Description: description,
			})
		}
	})

	for _, info := range mediaInfos {
		if info.Type == "audio" {
			doc.Find(fmt.Sprintf("audio source[src='%s']", info.URL)).AfterHtml(fmt.Sprintf("<p class='media-description'>Audio transcription: %s</p>", info.Description))
		} else if info.Type == "image" {
			doc.Find(fmt.Sprintf("img[src='%s']", info.URL)).AfterHtml(fmt.Sprintf("<p class='media-description'>Image description: %s</p>", info.Description))
		}
	}

	// Fetch questions
	arxivQuestionsURL := fmt.Sprintf("%s/data/%s/arxiv.txt", centralaBaseURL, centralaAPIKey)
	log.Printf("Fetching questions from %s", arxivQuestionsURL)
	resp, err = http.Get(arxivQuestionsURL)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch arxiv questions: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch arxiv questions: %v", err)})
		return
	}
	defer resp.Body.Close()

	questionsData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read arxiv questions: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read arxiv questions: %v", err)})
		return
	}

	questions := strings.Split(string(questionsData), "\n")
	var validQuestions []string
	for _, question := range questions {
		if question != "" {
			validQuestions = append(validQuestions, question)
		}
	}

	textContent := extractTextContent(doc)
	combinedPrompt := strings.Join(validQuestions, "\n") + "\n\nContent:\n" + textContent

	log.Printf("[DEBUG] Sending combined prompt to OpenAI (length: %d characters)", len(combinedPrompt))
	answer, err := openAIService.SendChatMessage(combinedPrompt)
	if err != nil {
		log.Printf("[ERROR] OpenAI API call failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get combined answer: %v", err)})
		return
	}
	log.Printf("[DEBUG] Received answer from OpenAI (length: %d characters)", len(answer))

	answers := make(map[string]string)
	answerLines := strings.Split(answer, "\n")
	for i, line := range answerLines {
		answers[fmt.Sprintf("%02d", i+1)] = strings.TrimSpace(line)
	}

	// Send report
	reportRequest := map[string]interface{}{
		"task":   "arxiv",
		"apikey": centralaAPIKey,
		"answer": answers,
	}

	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
	log.Printf("[INFO] Sending final report to: %s", reportURL)
	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		log.Printf("[ERROR] Report submission failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send report: %v", err)})
		return
	}

	log.Println("[INFO] Task8 completed successfully")
	ctx.JSON(http.StatusOK, gin.H{
		"combinedPrompt": combinedPrompt,
		"answers":        answers,
		"reportResponse": response,
	})
}
