package main

import (
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	r := gin.Default()

	r.GET("/ping", func(ctx *gin.Context) {
		log.Println("[INFO] Handling ping request")
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

	r.GET("/solveTask10", func(ctx *gin.Context) {
		tasks.SolveTask10(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask11", func(ctx *gin.Context) {
		tasks.SolveTask11(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	r.GET("/solveTask12", func(ctx *gin.Context) {
		tasks.SolveTask12(ctx, llmService, centralaBaseURL, centralaAPIKey)
	})

	return r
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := godotenv.Load()
	if err != nil {
		log.Fatal("[FATAL] Error loading .env file")
	}

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		log.Fatal("[FATAL] OLLAMA_URL not specified in environment variables")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("XYZ_BASE_URL")

	centralaBaseURL := os.Getenv("CENTRALA_BASE_URL")
	if centralaBaseURL == "" {
		log.Fatal("[FATAL] CENTRALA_BASE_URL environment variable is not set")
	}

	centralaAPIKey := os.Getenv("CENTRALA_API_KEY")
	if centralaAPIKey == "" {
		log.Fatal("[FATAL] CENTRALA_API_KEY environment variable is not set")
	}

	if baseURL == "" {
		log.Fatal("[FATAL] XYZ_BASE_URL environment variable is not set")
	}

	baseURL = strings.TrimRight(baseURL, "/")

	systemPrompt := `You are a helpful assistant that answers questions by providing street names. 
Return your answer in this format: { "question": "this is a question?", "answer": "street name" }. 
Be concise and return only the JSON response. 
<rule>NEVER USE MARKDOWN CODE BLOCKS</rule>
`
	llmService, err := services.NewOpenAIService(apiKey, systemPrompt, openai.GPT4oMini)
	if err != nil {
		log.Fatal("[FATAL] Error initializing LLM Service:", err)
	}

	r := setupRouter(llmService, baseURL, centralaBaseURL, centralaAPIKey, ollamaURL, apiKey)
	log.Println("[INFO] Starting server on :8080")
	r.Run(":8080")
}
