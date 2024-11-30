
package tasks

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask5(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
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