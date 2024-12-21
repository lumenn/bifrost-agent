package tasks

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

type DBResponse struct {
	Reply []map[string]interface{} `json:"reply"`
	Error string                   `json:"error"`
}

func SolveTask11(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	log.Println("[INFO] Starting Task11 execution")

	// Set up LLM for database exploration
	llmService.SetSystemPrompt(`You are a database expert. Your task is to analyze database structure and content.
		For each question, return only a SQL query as a plain string, without any markdown or additional text.
		The query should help explore the database structure or find specific information.
		Consider relationships between tables and use JOINs when necessary.`)

	// Initialize database explorer
	explorer := &DatabaseExplorer{
		baseURL: centralaBaseURL,
		apiKey:  centralaAPIKey,
		llm:     llmService,
	}

	// First, get the list of tables
	tables, err := explorer.listTables()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list tables: %v", err)})
		return
	}

	// Get table structures
	tableStructures := make(map[string]string)
	for _, table := range tables {
		structure, err := explorer.getTableStructure(table)
		if err != nil {
			log.Printf("[WARN] Failed to get structure for table %s: %v", table, err)
			continue
		}
		tableStructures[table] = structure
	}

	// Build context for LLM
	context := "Database structure:\n"
	for table, structure := range tableStructures {
		context += fmt.Sprintf("Table %s:\n%s\n\n", table, structure)
	}

	// Ask LLM for the query to find datacenters managed by workers on leave
	prompt := fmt.Sprintf(`%s

	Based on the database structure above, write a SQL query to find datacenter IDs that are managed by workers who are currently on leave.
	Consider:
	1. Look for tables related to workers/employees
	2. Look for tables related to datacenters
	3. Look for tables related to leave/absence status
	4. Join these tables appropriately

	Return only the SQL query, nothing else.`, context)

	query, err := llmService.SendChatMessage(prompt)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate query: %v", err)})
		return
	}

	// Execute the final query
	result, err := explorer.executeQuery(query)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to execute query: %v", err)})
		return
	}

	// Extract datacenter IDs from the result
	var datacenterIDs []int
	for _, row := range result {
		// Try to get the ID as a number (could be float64 from JSON)
		if id, ok := row["dc_id"].(float64); ok {
			datacenterIDs = append(datacenterIDs, int(id))
		} else if id, ok := row["dc_id"].(string); ok {
			// If it's a string, try to parse it as an integer
			if intID, err := strconv.Atoi(id); err == nil {
				datacenterIDs = append(datacenterIDs, intID)
			}
		}
	}

	// Send report to centrala
	reportRequest := map[string]interface{}{
		"task":   "database",
		"apikey": centralaAPIKey,
		"answer": datacenterIDs,
	}

	reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
	response, err := services.PostJSON(reportURL, reportRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send report: %v", err)})
		return
	}

	log.Println("[INFO] Task11 completed successfully")
	ctx.JSON(http.StatusOK, gin.H{
		"tables":         tables,
		"tableStructure": tableStructures,
		"finalQuery":     query,
		"result":         result,
		"datacenterIDs":  datacenterIDs,
		"reportRequest":  reportRequest,
		"reportResponse": response,
	})
}

type DatabaseExplorer struct {
	baseURL string
	apiKey  string
	llm     services.LLMService
}

func (e *DatabaseExplorer) executeQuery(query string) ([]map[string]interface{}, error) {
	request := map[string]interface{}{
		"task":   "database",
		"apikey": e.apiKey,
		"query":  query,
	}

	url := fmt.Sprintf("%s/apidb", e.baseURL)
	response, err := services.PostJSON(url, request)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	var dbResponse DBResponse
	if err := json.Unmarshal([]byte(response), &dbResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if dbResponse.Error != "OK" {
		return nil, fmt.Errorf("database error: %s", dbResponse.Error)
	}

	return dbResponse.Reply, nil
}

func (e *DatabaseExplorer) listTables() ([]string, error) {
	result, err := e.executeQuery("SHOW TABLES")
	if err != nil {
		return nil, err
	}

	var tables []string
	for _, row := range result {
		if tableName, ok := row["Tables_in_banan"].(string); ok {
			tables = append(tables, tableName)
		}
	}
	return tables, nil
}

func (e *DatabaseExplorer) getTableStructure(tableName string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE %s", tableName)
	result, err := e.executeQuery(query)
	if err != nil {
		return "", err
	}

	if len(result) == 0 {
		return "", fmt.Errorf("no structure returned for table %s", tableName)
	}

	// The structure is usually in the second column of the result
	for _, value := range result[0] {
		if structure, ok := value.(string); ok && strings.Contains(structure, "CREATE TABLE") {
			return structure, nil
		}
	}

	return "", fmt.Errorf("could not find table structure in result")
}
