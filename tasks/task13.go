package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	AccessLevel string `json:"access_level"`
	IsActive    string `json:"is_active"`
	LastLog     string `json:"lastlog"`
}

type Connection struct {
	User1ID string `json:"user1_id"`
	User2ID string `json:"user2_id"`
}

type PathResult struct {
	Path []string
}

func findShortestPath(ctx context.Context, session neo4j.SessionWithContext) ([]string, error) {
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH path = shortestPath((start:User {username: 'Rafał'})-[:CONNECTED_TO*]-(end:User {username: 'Barbara'}))
			RETURN [node IN nodes(path) | node.username] as path
		`
		result, err := tx.Run(ctx, query, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to execute query: %w", err)
		}

		record, err := result.Single(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get result: %w", err)
		}

		pathInterface, ok := record.Get("path")
		if !ok {
			return nil, fmt.Errorf("path not found in result")
		}

		pathSlice, ok := pathInterface.([]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid path format")
		}

		path := make([]string, len(pathSlice))
		for i, v := range pathSlice {
			path[i] = v.(string)
		}

		return path, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]string), nil
}

func SolveTask13(ctx *gin.Context, centralaBaseURL, centralaAPIKey string) {
	// Create CentralaService instance
	centralService := services.NewCentralaService(centralaBaseURL, centralaAPIKey, nil) // nil for openAIService as it's not needed

	// Initialize Neo4j driver
	ctxBg := context.Background()
	driver, err := neo4j.NewDriverWithContext(
		"neo4j://localhost:7687",
		neo4j.BasicAuth("neo4j", "your_password_here", ""), // Update with your Neo4j credentials
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create neo4j driver: %v", err)})
		return
	}
	defer driver.Close(ctxBg)

	// Verify connectivity
	err = driver.VerifyConnectivity(ctxBg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to connect to neo4j: %v", err)})
		return
	}

	// Fetch users from MySQL
	usersResponse, err := centralService.QueryDatabase("SELECT * FROM users")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch users: %v", err)})
		return
	}

	// Parse users
	var users []User
	usersData, err := services.InterfaceToJSON(usersResponse.Reply)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to convert users response: %v", err)})
		return
	}
	if err := services.JSONToStruct(usersData, &users); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse users: %v", err)})
		return
	}

	// Fetch connections
	connectionsResponse, err := centralService.QueryDatabase("SELECT * FROM connections")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch connections: %v", err)})
		return
	}

	// Parse connections
	var connections []Connection
	connectionsData, err := services.InterfaceToJSON(connectionsResponse.Reply)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to convert connections response: %v", err)})
		return
	}
	if err := services.JSONToStruct(connectionsData, &connections); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse connections: %v", err)})
		return
	}

	// Create Neo4j session
	session := driver.NewSession(ctxBg, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctxBg)

	// Clear existing data
	_, err = session.ExecuteWrite(ctxBg, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctxBg, "MATCH (n) DETACH DELETE n", nil)
		return nil, err
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to clear neo4j database: %v", err)})
		return
	}

	// Create users in Neo4j
	for _, user := range users {
		_, err = session.ExecuteWrite(ctxBg, func(tx neo4j.ManagedTransaction) (any, error) {
			params := map[string]any{
				"id":          user.ID,
				"username":    user.Username,
				"accessLevel": user.AccessLevel,
				"isActive":    user.IsActive,
				"lastLog":     user.LastLog,
			}
			query := `
				CREATE (u:User {
					id: $id,
					username: $username,
					accessLevel: $accessLevel,
					isActive: $isActive,
					lastLog: $lastLog
				})
			`
			_, err := tx.Run(ctxBg, query, params)
			return nil, err
		})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create user %s in neo4j: %v", user.Username, err)})
			return
		}
	}

	// Create connections in Neo4j
	for _, conn := range connections {
		_, err = session.ExecuteWrite(ctxBg, func(tx neo4j.ManagedTransaction) (any, error) {
			params := map[string]any{
				"user1Id": conn.User1ID,
				"user2Id": conn.User2ID,
			}
			query := `
				MATCH (u1:User {id: $user1Id})
				MATCH (u2:User {id: $user2Id})
				CREATE (u1)-[:CONNECTED_TO]->(u2)
			`
			_, err := tx.Run(ctxBg, query, params)
			return nil, err
		})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create connection %s->%s in neo4j: %v",
				conn.User1ID, conn.User2ID, err)})
			return
		}
	}

	log.Printf("Successfully imported %d users and %d connections to Neo4j",
		len(users), len(connections))

	// Find shortest path between Rafał and Barbara
	path, err := findShortestPath(ctxBg, session)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to find shortest path: %v", err)})
		return
	}

	// Create comma-separated string of usernames
	pathString := strings.Join(path, ", ")

	// Send report to centrala
	request := map[string]interface{}{
		"task":   "connections",
		"apikey": centralaAPIKey,
		"answer": pathString,
	}

	response, err := services.PostJSON(fmt.Sprintf("%s/report", centralaBaseURL), request)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to send report: %v", err)})
		return
	}

	// Parse response into JSON
	var jsonResponse interface{}
	if err := json.Unmarshal([]byte(response), &jsonResponse); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse response: %v", err)})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":      fmt.Sprintf("Successfully sent path report: %s", pathString),
		"request_body": request,
		"response":     jsonResponse,
	})
}
