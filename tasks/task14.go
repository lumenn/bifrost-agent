package tasks

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

type AnalyzedImage struct {
	URL             string
	FilePath        string
	Filename        string
	Description     string
	UsedDarken      bool
	DarkenedImage   *AnalyzedImage
	UsedRepair      bool
	RepairedImage   *AnalyzedImage
	UsedBrighten    bool
	BrightenedImage *AnalyzedImage
}

type LLMResponse struct {
	Thinking    string   `json:"thinking"`
	Description string   `json:"description"`
	NextTool    string   `json:"nextTool"`
	Filenames   []string `json:"filenames"`
}

func getFilename(img *AnalyzedImage) string {
	if img != nil {
		return img.Filename
	}
	return "Not Available"
}

func String(images map[string]*AnalyzedImage) string {
	result := ""
	for _, image := range images {
		result += fmt.Sprintf(`
		Filename: %s
		- Description: %s
		- Used Darken: %t
		- Image after darken: %s
		- Used Repair: %t
		- Image after repair: %s
		- Used Brighten: %t
		- Image after brighten: %s
		`, image.Filename, image.Description,
			image.UsedDarken, getFilename(image.DarkenedImage),
			image.UsedRepair, getFilename(image.RepairedImage),
			image.UsedBrighten, getFilename(image.BrightenedImage))
	}
	return result
}

func Merge(images map[string]*AnalyzedImage, newImages map[string]*AnalyzedImage) map[string]*AnalyzedImage {
	merged := map[string]*AnalyzedImage{}
	for k, v := range images {
		if merged[k] == nil {
			merged[k] = v
		}
	}
	for k, v := range newImages {
		if merged[k] == nil {
			merged[k] = v
		}
	}
	return merged
}

func getFirstKey(m map[string]*AnalyzedImage) string {
	for k := range m {
		return k
	}
	return ""
}

func SolveTask14(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	os.MkdirAll("/tmp/task14", 0755)
	openAIService, ok := llmService.(*services.OpenAiService)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "LLM service must be OpenAI"})
		return
	}

	centralaService := services.NewCentralaService(centralaBaseURL, centralaAPIKey, openAIService)

	report, err := centralaService.PostReport("photos", "START")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	llmService.SetSystemPrompt(`
		You'r job is to analyze people in the photos and find person common between multiple pictures and create detailed description of that person.
		Only possible images are specified in <images> tag.
		To do your job you can use tools as below:
			- DARKEN - to darken the image (single image)
			- REPAIR - to repair the image (single image)
			- BRIGHTEN - to brighten the image (single image)
			- DESCRIBE - to prepare a description of picture (single image)
			- CHECK - to prepare a description of barbara and check if it is correct (multiple images)
		Return the response in JSON format as below, do not use markdown:
		{
			"thinking": "Describe thought process on why specific tool should be used, and which parts of the image suggest that tool",
			"description": "Detailed description of people on the image, or any damages/glitches on the image.",
			"nextTool": "Name of the tool to use next",
			"filenames": ["filename1.ext"]
		}
		Rules:
			- You can use only one tool at a time.
			- DO NOT USE MARKDOWN IN YOUR RESPONSE
			- Only CHECK tool can be used with multiple images.
			- If you are asked to use specific language - use it.
		`)

	imageFiles := extractFiles(report.Message, centralaBaseURL)
	reasoningHistory := make([]string, 0)
	hints := []string{}
	iteration := 0
	for {
		iteration++

		// Get last 15 entries from reasoning history in FIFO order (latest first)
		historyLen := len(reasoningHistory)
		start := historyLen - 30
		if start < 0 {
			start = 0
		}
		recentHistory := reasoningHistory[start:historyLen]

		response, err := openAIService.SendChatMessage(fmt.Sprintf(`
		Given the following images and data about them indentify Barbara.
		I encourage you to use each tool on each image - this will help you to find Barbara.
		<images>
		%s
		</images>

		<reasoning_history>
		%s
		</reasoning_history>

		<hints>
		%s
		</hints>
		`, String(imageFiles), strings.Join(recentHistory, "\n"), strings.Join(hints, ", ")))

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		llmResponse := LLMResponse{}
		cleanedResponse := clearFromMarkdown(response)
		err = json.Unmarshal([]byte(cleanedResponse), &llmResponse)
		if err != nil {
			log.Printf("[ERROR] Failed to unmarshal LLM response. Original: %s\nCleaned: %s\nError: %v",
				response, cleanedResponse, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: Reasoning: %s", iteration, llmResponse.Description))

		switch llmResponse.NextTool {
		case "DARKEN":
			darkenResponse, err := centralaService.PostReport("photos", fmt.Sprintf("DARKEN %s", llmResponse.Filenames[0]))
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			newImages := extractFiles(darkenResponse.Message, centralaBaseURL)
			imageFiles = Merge(imageFiles, newImages)

			reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: DARKEN %s\n%s", iteration, llmResponse.Filenames[0], darkenResponse.Message))

			imageFiles[llmResponse.Filenames[0]].UsedDarken = true
			imageFiles[llmResponse.Filenames[0]].DarkenedImage = newImages[getFirstKey(newImages)]
			continue
		case "REPAIR":
			repairResponse, err := centralaService.PostReport("photos", fmt.Sprintf("REPAIR %s", llmResponse.Filenames[0]))
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			newImages := extractFiles(repairResponse.Message, centralaBaseURL)
			imageFiles = Merge(imageFiles, newImages)

			reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: REPAIR %s\n%s", iteration, llmResponse.Filenames[0], repairResponse.Message))

			imageFiles[llmResponse.Filenames[0]].UsedRepair = true
			imageFiles[llmResponse.Filenames[0]].RepairedImage = newImages[getFirstKey(newImages)]
			continue
		case "BRIGHTEN":
			brightenResponse, err := centralaService.PostReport("photos", fmt.Sprintf("BRIGHTEN %s", llmResponse.Filenames[0]))
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			newImages := extractFiles(brightenResponse.Message, centralaBaseURL)
			imageFiles = Merge(imageFiles, newImages)

			reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: BRIGHTEN %s\n%s", iteration, llmResponse.Filenames[0], brightenResponse.Message))

			imageFiles[llmResponse.Filenames[0]].UsedBrighten = true
			imageFiles[llmResponse.Filenames[0]].BrightenedImage = newImages[getFirstKey(newImages)]
			continue
		case "DESCRIBE":
			describeResponse, err := openAIService.AnalyzeImages(`Provide detailed description of Barbara, please focus on: `+strings.Join(hints, ", "), imageFiles[llmResponse.Filenames[0]].FilePath)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			unmarshaledDescribeResponse := LLMResponse{}
			cleanedDescribeResponse := clearFromMarkdown(describeResponse)
			unmarshalErr := json.Unmarshal([]byte(cleanedDescribeResponse), &unmarshaledDescribeResponse)
			if unmarshalErr != nil {
				log.Printf("[ERROR] Failed to unmarshal describe response. Original: %s\nCleaned: %s\nError: %v",
					describeResponse, cleanedDescribeResponse, unmarshalErr)
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": unmarshalErr.Error()})
				return
			}

			imageFiles[llmResponse.Filenames[0]].Description = unmarshaledDescribeResponse.Description
			reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: DESCRIBE %s\n%s", iteration, llmResponse.Filenames[0], unmarshaledDescribeResponse.Description))
			continue
		case "CHECK":
			filePaths := []string{}
			for _, filename := range llmResponse.Filenames {
				filePaths = append(filePaths, imageFiles[filename].FilePath)
			}

			checkResponse, err := openAIService.AnalyzeImages(fmt.Sprintf(`
			Przygotuj dokładny opis postaci w języku Polskim, skup się w szczególności na cechach wyróżniających, %s`, strings.Join(report.Hints, ", ")), filePaths...)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			//translate to polish using llm
			translateResponse, err := openAIService.SendChatMessage(fmt.Sprintf(`Translate the following text to Polish, return only the translated text, do not change the json structure - translate only description: %s`, checkResponse))
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			unmarshaledCheckResponse := LLMResponse{}
			cleanedTranslateResponse := clearFromMarkdown(translateResponse)
			unmarshalErr := json.Unmarshal([]byte(cleanedTranslateResponse), &unmarshaledCheckResponse)
			if unmarshalErr != nil {
				log.Printf("[ERROR] Failed to unmarshal check response. Original: %s\nCleaned: %s\nError: %v",
					checkResponse, cleanedTranslateResponse, unmarshalErr)
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": unmarshalErr.Error()})
				return
			}

			centralaResponse, err := centralaService.PostReport("photos", unmarshaledCheckResponse.Description)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			if strings.Contains(centralaResponse.Message, "{{FLG:") {
				ctx.JSON(http.StatusOK, gin.H{"description": centralaResponse.Message})
				return
			}

			newImages := extractFiles(centralaResponse.Message, centralaBaseURL)
			imageFiles = Merge(imageFiles, newImages)

			hints = centralaResponse.Hints

			reasoningHistory = append(reasoningHistory, fmt.Sprintf("Iteration %d: CHECK %s\nReceived description: %s\nReceived hints: %s",
				iteration, strings.Join(llmResponse.Filenames, ", "), unmarshaledCheckResponse.Description, strings.Join(hints, ", ")))
			continue
		}
	}
}

func extractFiles(report string, centralaBaseURL string) map[string]*AnalyzedImage {
	re := regexp.MustCompile(`IMG_\d+(_[A-Z0-9]+)?`)
	matches := re.FindAllString(report, -1)
	images := map[string]*AnalyzedImage{}

	for _, match := range matches {
		fileName := match + "-small.png"
		url := fmt.Sprintf("%s/dane/barbara/%s", centralaBaseURL, fileName)
		filePath := fmt.Sprintf("/tmp/task14/%s", fileName)
		err := services.DownloadFile(url, filePath)
		if err != nil {
			continue
		}

		images[fileName] = &AnalyzedImage{
			Filename:     fileName,
			URL:          url,
			FilePath:     filePath,
			Description:  "",
			UsedDarken:   false,
			UsedRepair:   false,
			UsedBrighten: false,
		}
	}

	return images
}

func clearFromMarkdown(text string) string {
	re := regexp.MustCompile(`\` + "`" + `json\s*` + "`" + `\s*`)
	return re.ReplaceAllString(text, "")
}
