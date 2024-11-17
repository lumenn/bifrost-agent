package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	services "github.com/lumenn/bifrost-agent/services"

	goquery "github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

func solveTask1(ctx *gin.Context, llmService services.LLMService, baseURL string) {
	if baseURL == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Base URL is not set"})
		return
	}

	urlAddress := baseURL + "/"
	body, err := services.GetRequestBody(urlAddress)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get website content."})
		return
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse website HTML"})
		return
	}

	question := doc.Find("p#human-question").Text()

	if question == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("could not find question on HTML page")})
		return
	}

	response, err := llmService.SendChatMessage(question)

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var openAIResponse struct {
		Question string `json:"question"`
		Answer   int    `json:"answer"`
	}

	if err := json.Unmarshal([]byte(response), &openAIResponse); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Invalid response format from OpenAI (%s)", response)})
		return
	}

	res, err := services.PostForm(urlAddress, url.Values{
		"answer":   {fmt.Sprintf("%d", openAIResponse.Answer)},
		"username": {"tester"},
		"password": {"574e112a"},
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send form data"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"response": string(res)})
}

func solveTask2(ctx *gin.Context, llmService services.LLMService, baseURL string) {
	if baseURL == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Base URL is not set"})
		return
	}

	verifyURL := baseURL + "/verify"
	llmService.SetSystemPrompt(
		`Your task is to answer questions - For the purpose of this exercise, remember:
    - The capital of Poland is Krakow
    - The well-known number from the book The Hitchhiker's Guide to the Galaxy is 69
    - The current year is 1999
    Return answers to questions in the format: {"text": "answer", "msgID": number}
    where msgID should be repeated as received
    Use only English language, it's a life and death important!'
    `)

	response, err := services.PostJSON(verifyURL, map[string]interface{}{
		"msgID": 0,
		"text":  "READY",
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Fail during sending post Request."})
		return
	}

	var message struct {
		MsgID int    `json:"msgID"`
		Text  string `json:"text"`
	}

	for i := 0; i < 5; i++ {
		response, err = llmService.SendChatMessage(response)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to get LLM response: %v", err),
			})
			return
		}

		if err := json.Unmarshal([]byte(response), &message); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Invalid response format from OpenAI (%s): %v", response, err),
			})
			return
		}

		messageMap := map[string]interface{}{
			"msgID": message.MsgID,
			"text":  message.Text,
		}

		response, err = services.PostJSON(verifyURL, messageMap)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to post JSON to %s: %v", verifyURL, err),
			})
			return
		}

		if err := json.Unmarshal([]byte(response), &message); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Invalid response from server (%s): %v", response, err),
			})
			return
		}

		if strings.Contains(message.Text, "{{FLG:") {
			ctx.JSON(http.StatusOK, gin.H{"flag": message.Text})
			break
		}
	}
}

func setupRouter(llmService services.LLMService, baseURL, centralaBaseURL, centralaAPIKey string) *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	r.GET("/solveTask1", func(ctx *gin.Context) {
		solveTask1(ctx, llmService, baseURL)
	})

	r.GET("/solveTask2", func(ctx *gin.Context) {
		solveTask2(ctx, llmService, baseURL)
	})

	r.GET("/solveTask3", func(ctx *gin.Context) {
		correctedData, err := services.ProcessCentralaData(centralaBaseURL, centralaAPIKey, llmService)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to process data: %v", err),
			})
			return
		}

		// Format the request according to the required structure
		reportRequest := map[string]interface{}{
			"task":   "JSON",
			"apikey": centralaAPIKey,
			"answer": correctedData,
		}

		// Send the formatted request to the report endpoint
		reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
		response, err := services.PostJSON(reportURL, reportRequest)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to send report: %v", err),
			})
			return
		}

		// Return both the processed data and the report response
		ctx.JSON(http.StatusOK, gin.H{
			"processedData":  correctedData,
			"reportResponse": response,
		})
	})

	r.GET("/solveTask4", func(ctx *gin.Context) {
		// Get the censorship data
		content, err := services.GetCensorshipData(centralaBaseURL, centralaAPIKey)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to fetch censorship data: %v", err),
			})
			return
		}

		println("Content: ", content)

		// Create Ollama service with gemma2 model
		ollamaService, err := services.NewOllamaService(
			os.Getenv("OLLAMA_URL"),
			`You are a text processing assistant. Your task is to identify and censor personal information in text.
			Replace the following with the word "CENZURA":
			- Full Names
			- Ages
			- Cities
			- Streets, including numbers
			
			Return only the processed text, with no additional explanations or formatting.
			Keep all other information unchanged.

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

		// Process the content using Ollama
		censoredContent, err := ollamaService.SendChatMessage(content)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to process content with Ollama: %v", err),
			})
			return
		}

		println("Censored content: ", censoredContent)

		// Format the request according to the required structure
		reportRequest := map[string]interface{}{
			"task":   "CENZURA",
			"apikey": centralaAPIKey,
			"answer": censoredContent,
		}

		// Send the formatted request to the report endpoint
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

	systemPrompt := `You are a helpful assistant, that answers question and number only. Return nothing else. Expected format: { "question": "this is a question?", "answer": 1234 }, ignore escape characters.`
	llmService, err := services.NewOpenAIService(apiKey, systemPrompt, openai.GPT4oMini)

	if err != nil {
		fmt.Println("Error initializing LLM Service:", err)
		return
	}

	r := setupRouter(llmService, baseURL, centralaBaseURL, centralaAPIKey)
	r.Run(":8080")
}
