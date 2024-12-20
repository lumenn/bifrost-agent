package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask1(ctx *gin.Context, llmService services.LLMService, baseURL string) {
	if baseURL == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Base URL is not set"})
		return
	}

	llmService.SetSystemPrompt("Please provide answer to given question only, using format: { \"question\": \"question\", \"answer\": \"answer\" }")

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
