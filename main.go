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
	"github.com/lumenn/bifrost-agent/tasks"

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

func solveTask3(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
    correctedData, err := services.ProcessCentralaData(centralaBaseURL, centralaAPIKey, llmService)
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": fmt.Sprintf("Failed to process data: %v", err),
        })
        return
    }

    reportRequest := map[string]interface{}{
        "task":   "JSON",
        "apikey": centralaAPIKey,
        "answer": correctedData,
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
        "processedData":  correctedData,
        "reportResponse": response,
    })
}

func solveTask4(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey, ollamaURL string) {
    content, err := services.GetCensorshipData(centralaBaseURL, centralaAPIKey)
    if (err != nil) {
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
    if (err != nil) {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": fmt.Sprintf("Failed to create Ollama service: %v", err),
        })
        return
    }

    censoredContent, err := ollamaService.SendChatMessage(content)
    if (err != nil) {
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
    if (err != nil) {
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

func solveTask5(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
    openAIService, ok := llmService.(*services.OpenAiService)
    if (!ok) {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": "LLM service is not an OpenAI service",
        })
        return
    }

    transcriptions, err := openAIService.TranscribeDirectory("datasets/task5")
    if (err != nil) {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": fmt.Sprintf("Failed to transcribe audio files: %v", err),
        })
        return
    }

    var combinedText string
    for _, transcription := range transcriptions {
        combinedText += transcription + "\n"
    }

    prompt := `Please analyze these transcriptions carefully. Think step by step:
    Think slowly and carefully.
    Think about any locations which might be connected to universities or educational institutions.
    They might not be directly mentioned.
    Make sure to use external sources and your knowledge to find the answer.
    Is there any specific part of university which is mentioned?

    Return me only the answer in this format: { "question": "On which street is the university where Andrzej Maj gives lectures?", "answer": "street name" , "possible_locations": [{"name": "street name1", "reasoning": "reasoning1", "confidence": 0.8}, {"name": "street name2", "reasoning": "reasoning2", "confidence": 0.5}, {"name": "street name3", "reasoning": "reasoning3", "confidence": 0.3}]}
    Correct answer is not given in the transcriptions, but it's possible to connect the dots.

    Transcriptions:
    ` + combinedText

    response, err := openAIService.SendChatMessage(prompt)
    if (err != nil) {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": fmt.Sprintf("Failed to process with GPT: %v", err),
        })
        return
    }

    reportRequest := map[string]interface{}{
        "task":   "mp3",
        "apikey": centralaAPIKey,
        "answer": response,
    }

    reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
    reportResponse, err := services.PostJSON(reportURL, reportRequest)
    if (err != nil) {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": fmt.Sprintf("Failed to send report: %v", err),
        })
        return
    }

    ctx.JSON(http.StatusOK, gin.H{
        "transcriptions": transcriptions,
        "gptResponse":    response,
        "reportResponse": reportResponse,
    })
}

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
Be concise and return only the JSON response.`
	llmService, err := services.NewOpenAIService(apiKey, systemPrompt, openai.GPT4)

	if err != nil {
		fmt.Println("Error initializing LLM Service:", err)
		return
	}

	r := setupRouter(llmService, baseURL, centralaBaseURL, centralaAPIKey, ollamaURL, apiKey)
	r.Run(":8080")
}
