
package tasks

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

func SolveTask3(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
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