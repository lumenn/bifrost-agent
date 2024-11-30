package tasks

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask4(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey, ollamaURL string) {
	openAIService, ok := llmService.(*services.OpenAiService)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "LLM service is not an OpenAI service",
		})
		return
	}

	centralaService := services.NewCentralaService(centralaBaseURL, centralaAPIKey, openAIService)
	content, err := centralaService.GetCensorshipData()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch censorship data: %v", err),
		})
		return
	}

	ollamaService, err := services.NewOllamaService(
		ollamaURL,
		`You are a text processing assistant. Your task is to identify and censor personal information in text.
        Replace the following with the word "CENZURA":
        - Full Names
        - Ages
        - Cities
        - Streets, including numbers
        
        Return only the processed text, with no additional explanations or formatting.
        Keep all other information unchanged..

		Examples:
		- "Bartosz Przykładowy" -> "CENZURA"
		- "I live at ul. Mickiewicza 15 in Warsaw" -> "I live at ul. CENZURA in CENZURA"
		- "Address: st. Oak Street 45, Chicago" -> "Address: st. CENZURA, CENZURA"
		- "Contact Sarah Jones, age 30, at ul. Długa 7" -> "Contact CENZURA, age CENZURA, at ul. CENZURA"
		`,
		"gemma2",
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create Ollama service: %v", err),
		})
		return
	}

	censoredContent, err := ollamaService.SendChatMessage(content)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to process content with Ollama: %v", err),
		})
		return
	}

	reportRequest := map[string]interface{}{
		"task":   "CENZURA",
		"apikey": centralaAPIKey,
		"answer": censoredContent,
	}

	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to send report: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"originalContent": content,
		"censoredContent": censoredContent,
		"reportResponse":  response,
	})
}
