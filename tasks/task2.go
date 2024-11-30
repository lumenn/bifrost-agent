
package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask2(ctx *gin.Context, llmService services.LLMService, baseURL string) {
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