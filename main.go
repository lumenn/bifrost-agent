package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	services "github.com/lumenn/bifrost-agent/services"
	"github.com/lumenn/bifrost-agent/tasks"
	"github.com/sashabaranov/go-openai"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func setupRouter(llmService services.LLMService, baseURL, centralaBaseURL, centralaAPIKey, ollamaURL, openaiAPIKey string) *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	r.GET("/solveTask1", func(ctx *gin.Context) {
		tasks.SolveTask1(ctx, llmService, baseURL)
	})

	r.GET("/solveTask2", func(ctx *gin.Context) {
		tasks.SolveTask2(ctx, llmService, baseURL)
	})

	r.GET("/solveTask3", func(ctx *gin.Context) {
		tasks.SolveTask3(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask4", func(ctx *gin.Context) {
		tasks.SolveTask4(ctx, llmService, centralaBaseURL, centralaAPIKey, ollamaURL)
	})

	r.GET("/solveTask5", func(ctx *gin.Context) {
		tasks.SolveTask5(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask6", func(ctx *gin.Context) {
		tasks.SolveTask6(ctx, centralaBaseURL, centralaAPIKey, openaiAPIKey)
	})

	r.GET("/solveTask7", func(ctx *gin.Context) {
		tasks.SolveTask7(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask8", func(ctx *gin.Context) {
		tasks.SolveTask8(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask9", func(ctx *gin.Context) {
		tasks.SolveTask9(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	return r
}

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Get environment variables
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		log.Fatal("OLLAMA_URL not specified in environment variables")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("XYZ_BASE_URL")

	// Add Centrala environment variables here
	centralaBaseURL := os.Getenv("CENTRALA_BASE_URL")
	if centralaBaseURL == "" {
		fmt.Println("Error: CENTRALA_BASE_URL environment variable is not set")
		return
	}

	centralaAPIKey := os.Getenv("CENTRALA_API_KEY")
	if centralaAPIKey == "" {
		fmt.Println("Error: CENTRALA_API_KEY environment variable is not set")
		return
	}

	if baseURL == "" {
		fmt.Println("Error: XYZ_BASE_URL environment variable is not set")
		return
	}

	// Trim any trailing slashes from the base URL
	baseURL = strings.TrimRight(baseURL, "/")

	systemPrompt := `You are a helpful assistant that answers questions by providing street names. 
Return your answer in this format: { "question": "this is a question?", "answer": "street name" }. 
Be concise and return only the JSON response. 
<rule>NEVER USE MARKDOWN CODE BLOCKS</rule>
`
	// llmService, err := services.NewOllamaService(ollamaURL, systemPrompt, "gemma2")
	llmService, err := services.NewOpenAIService(apiKey, systemPrompt, openai.GPT4oMini)

	if err != nil {
		fmt.Println("Error initializing LLM Service:", err)
		return
	}

	r := setupRouter(llmService, baseURL, centralaBaseURL, centralaAPIKey, ollamaURL, apiKey)
	r.Run(":8080")
}
