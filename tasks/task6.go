package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
	openai "github.com/sashabaranov/go-openai"
)

type RobotDescription struct {
	Description string `json:"description"`
}

func SolveTask6(ctx *gin.Context, centralaBaseURL, centralaAPIKey, openaiAPIKey string) {
	// Get robot description from centrala
	robotDescURL := fmt.Sprintf("%s/data/%s/robotid.json", centralaBaseURL, centralaAPIKey)
	resp, err := http.Get(robotDescURL)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch robot description: %v", err)})
		return
	}
	defer resp.Body.Close()

	var robotDesc RobotDescription
	if err := json.NewDecoder(resp.Body).Decode(&robotDesc); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to decode robot description: %v", err)})
		return
	}

	// Generate image using DALLE
	client := openai.NewClient(openaiAPIKey)
	imageResp, err := client.CreateImage(ctx, openai.ImageRequest{
		Prompt:         robotDesc.Description,
		Size:          openai.CreateImageSize1024x1024,
		Model:         openai.CreateImageModelDallE3,
		N:             1,
		ResponseFormat: openai.CreateImageResponseFormatURL,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate image: %v", err)})
		return
	}

	// Send report to centrala
	reportRequest := map[string]interface{}{
		"task":   "robotid",
		"apikey": centralaAPIKey,
		"answer": imageResp.Data[0].URL,
	}

	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send report: %v", err)})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"robotDescription": robotDesc.Description,
		"generatedImageURL": imageResp.Data[0].URL,
		"reportResponse": response,
	})
}