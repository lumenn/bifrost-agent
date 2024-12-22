package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

type APIResponse struct {
	Reply interface{} `json:"reply"`
	Error string      `json:"error"`
}

type ConnectionInfo struct {
	Links   []string // places visited by person or people who visited place
	Queried bool     // whether we've queried this entity
}

type ConnectionMap struct {
	PeopleToPlaces  map[string]ConnectionInfo // person -> places visited and query status
	PlacesToPeople  map[string]ConnectionInfo // place -> people who visited and query status
	WrongGuesses    map[string]bool           // track which places were incorrect guesses
	ReasoningLog    []string                  // track reasoning history
	UnqueriedPeople []string                  // people discovered but not yet queried
	UnqueriedPlaces []string                  // places discovered but not yet queried
}

type LLMDecision struct {
	Action    string `json:"action"`    // "ask_places", "ask_people", or "reason"
	Query     string `json:"query"`     // the name to query if action is ask_places or ask_people
	Reasoning string `json:"reasoning"` // explanation for the decision
	Answer    string `json:"answer"`    // city name to try if action is "answer"
}

// Helper function to remove Polish diacritics
func removeDiacritics(s string) string {
	replacements := map[string]string{
		"ą": "a", "ć": "c", "ę": "e", "ł": "l", "ń": "n", "ó": "o", "ś": "s", "ź": "z", "ż": "z",
		"Ą": "A", "Ć": "C", "Ę": "E", "Ł": "L", "Ń": "N", "Ó": "O", "Ś": "S", "Ź": "Z", "Ż": "Z",
	}
	result := s
	for from, to := range replacements {
		result = strings.ReplaceAll(result, from, to)
	}
	return result
}

// Helper function to normalize names/places (remove diacritics and convert to uppercase)
func normalizeString(s string) string {
	return strings.ToUpper(strings.TrimSpace(removeDiacritics(s)))
}

// Helper function to get first name from a full name
func getFirstName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) > 0 {
		return parts[0]
	}
	return fullName
}

func SolveTask12(ctx *gin.Context, llmService services.LLMService, centralaBaseURL, centralaAPIKey string) {
	log.Println("[DEBUG] Starting Task12 execution with centralaBaseURL:", centralaBaseURL)

	centralaService := services.NewCentralaService(centralaBaseURL, centralaAPIKey, nil)
	connections := ConnectionMap{
		PeopleToPlaces:  make(map[string]ConnectionInfo),
		PlacesToPeople:  make(map[string]ConnectionInfo),
		WrongGuesses:    make(map[string]bool),
		ReasoningLog:    make([]string, 0),
		UnqueriedPeople: make([]string, 0),
		UnqueriedPlaces: make([]string, 0),
	}
	log.Println("[DEBUG] Initialized connection maps")

	// Download the note
	log.Printf("[DEBUG] Attempting to download note from: %s/dane/barbara.txt", centralaBaseURL)
	noteContent, err := downloadNote(fmt.Sprintf("%s/dane/barbara.txt", centralaBaseURL))
	if err != nil {
		log.Printf("[ERROR] Failed to download note: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to download note: %v", err)})
		return
	}
	log.Printf("[DEBUG] Successfully downloaded note (%d bytes)", len(noteContent))

	// Set system prompt for the investigation
	llmService.SetSystemPrompt(`You are a detective investigating Barbara's location. You have access to these tools:
		1. /people endpoint - returns places visited by a person (input: FIRST NAME ONLY in uppercase, without Polish diacritics)
		2. /places endpoint - returns people who visited a place (input: place name in uppercase, without diacritics)
		3. answer - try to determine Barbara's location based on collected evidence

		IMPORTANT RULES ABOUT DATA ACCESS:
		- Once you query a person or place, you can NEVER query it again - the data is restricted
		- Each person/place can only be queried ONCE in the entire investigation
		- There is NO WAY to get additional information about already queried entities
		- Focus on exploring new connections through unqueried people and places
		- RESTRICTED DATA means there is no way to get the information.
		- CITY NAMES are PLACES.
		For each step, analyze the available information and decide what to do next. Return your decision in JSON format:
		{
			"action": "ask_places" | "ask_people" | "reason" | "answer",
			"query": "NAME_OR_PLACE_TO_QUERY",
			"reasoning": "explanation for this decision",
			"answer": "CITY_NAME if action is answer"
		}

		Remember:
		- Names must be in uppercase WITHOUT Polish diacritics:
		  - ą -> a, ć -> c, ę -> e, ł -> l, ń -> n, ó -> o, ś -> s, ź/ż -> z
		  - Examples: JOZEF, LUKASZ, MALGORZATA
		- When querying /people, use FIRST NAME ONLY (e.g., "BARBARA" not "BARBARA KOWALSKA")
		- Consider connections between people and places
		- When you have a theory about Barbara's location, don't hesitate to use "answer" action
		- Each wrong answer helps narrow down the possibilities
		- Be confident in your deductions - if you see a pattern, try answering!
		- Use "reason" action to analyze current information and plan next steps
		- Feel free to query any new names or places you discover during the investigation
		- DO NOT suggest places that were already marked as incorrect
		- Pay special attention to the unqueried_entities section - these are your opportunities for new information
		DO NOT USE MARKDOWN OR ANY OTHER FORMATTING.`)

	// Start the investigation loop
	var foundFlag bool
	maxSteps := 200 // prevent infinite loops
	log.Printf("[DEBUG] Starting investigation loop with maximum %d steps", maxSteps)

	for steps := 0; steps < maxSteps && !foundFlag; steps++ {
		// Force an answer attempt every 10 steps
		forceAnswer := steps > 0 && steps%10 == 0
		if forceAnswer {
			log.Printf("[DEBUG] Step %d: Forcing answer attempt", steps+1)
		}

		log.Printf("[DEBUG] Step %d/%d - Building state YAML", steps+1, maxSteps)
		log.Printf("[DEBUG] Current connections - People: %d, Places: %d",
			len(connections.PeopleToPlaces), len(connections.PlacesToPeople))

		// Prepare the current state for LLM
		stateYAML := fmt.Sprintf(`
Current state:
%s
  note: |
    %s

  connections:
    people_to_places:
%s
    places_to_people:
%s

  discovered_unqueried:
    people:
%s
    places:
%s

  queried:
    wrong_guesses:
%s
`,
			func() string {
				if forceAnswer {
					return "  IMPORTANT: You must use 'answer' action this turn - make your best guess based on current information!\n"
				}
				return ""
			}(),
			noteContent,
			formatConnectionsToYAML(connections.PeopleToPlaces),
			formatConnectionsToYAML(connections.PlacesToPeople),
			formatArrayToYAML(connections.UnqueriedPeople),
			formatArrayToYAML(connections.UnqueriedPlaces),
			formatQueriedToYAML(connections.WrongGuesses))

		// Ask LLM for next action
		decisionPrompt := fmt.Sprintf("Based on the current state, what should we do next?\n%s", stateYAML)
		log.Printf("[DEBUG] Sending decision prompt to LLM (prompt length: %d)", len(decisionPrompt))
		log.Printf("[DEBUG] Full prompt:\n%s", decisionPrompt)
		response, err := llmService.SendChatMessage(decisionPrompt)
		if err != nil {
			log.Printf("[ERROR] LLM request failed: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get LLM decision: %v", err)})
			return
		}
		log.Printf("[DEBUG] Received LLM response (length: %d):\n%s", len(response), response)

		var decision LLMDecision
		if err := json.Unmarshal([]byte(response), &decision); err != nil {
			log.Printf("[ERROR] Failed to parse LLM response '%s': %v", response, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse LLM response: %v", err)})
			return
		}

		log.Printf("[INFO] Step %d - Action: %s, Query: %s", steps+1, decision.Action, decision.Query)
		log.Printf("[INFO] Reasoning: %s", decision.Reasoning)

		switch decision.Action {
		case "ask_people":
			connections.ReasoningLog = append(connections.ReasoningLog,
				fmt.Sprintf("Step %d: Querying person %s - %s",
					steps+1, normalizeString(getFirstName(decision.Query)), decision.Reasoning))
			normalizedPerson := normalizeString(getFirstName(decision.Query))
			log.Printf("[DEBUG] Normalized person name: %s -> %s", decision.Query, normalizedPerson)
			if info, exists := connections.PeopleToPlaces[normalizedPerson]; exists && info.Queried {
				log.Printf("[DEBUG] Skipping already queried person: %s", normalizedPerson)
				continue
			}
			removeFromUnqueried(normalizedPerson, &connections.UnqueriedPeople)

			log.Printf("[DEBUG] Querying /people endpoint for: %s", normalizedPerson)
			data, err := centralaService.QueryAPI("/people", normalizedPerson)
			if err != nil {
				log.Printf("[WARN] Failed to query person data for %s: %v", normalizedPerson, err)
				continue
			}

			places := strings.Fields(data.(services.EntityResponse).Message)
			log.Printf("[DEBUG] Received %d places for person %s", len(places), normalizedPerson)
			normalizedPlaces := make([]string, 0, len(places))
			for _, place := range places {
				normalizedPlace := normalizeString(place)
				normalizedPlaces = append(normalizedPlaces, normalizedPlace)
				addUnqueriedEntity(normalizedPlace, connections.PlacesToPeople, &connections.UnqueriedPlaces)
			}
			connections.PeopleToPlaces[normalizedPerson] = ConnectionInfo{
				Links:   normalizedPlaces,
				Queried: true,
			}
			log.Printf("[DEBUG] Updated connections for %s with places: %v", normalizedPerson, normalizedPlaces)

		case "ask_places":
			connections.ReasoningLog = append(connections.ReasoningLog,
				fmt.Sprintf("Step %d: Querying place %s - %s",
					steps+1, normalizeString(decision.Query), decision.Reasoning))
			normalizedPlace := normalizeString(decision.Query)
			log.Printf("[DEBUG] Normalized place name: %s -> %s", decision.Query, normalizedPlace)
			if info, exists := connections.PlacesToPeople[normalizedPlace]; exists && info.Queried {
				log.Printf("[DEBUG] Skipping already queried place: %s", normalizedPlace)
				continue
			}
			removeFromUnqueried(normalizedPlace, &connections.UnqueriedPlaces)

			log.Printf("[DEBUG] Querying /places endpoint for: %s", normalizedPlace)
			data, err := centralaService.QueryAPI("/places", normalizedPlace)
			if err != nil {
				log.Printf("[WARN] Failed to query place data for %s: %v", normalizedPlace, err)
				continue
			}

			people := strings.Fields(data.(services.EntityResponse).Message)
			log.Printf("[DEBUG] Received %d people for place %s", len(people), normalizedPlace)
			normalizedPeople := make([]string, 0, len(people))
			for _, person := range people {
				normalizedPerson := normalizeString(getFirstName(person))
				normalizedPeople = append(normalizedPeople, normalizedPerson)
				addUnqueriedEntity(normalizedPerson, connections.PeopleToPlaces, &connections.UnqueriedPeople)
			}
			connections.PlacesToPeople[normalizedPlace] = ConnectionInfo{
				Links:   normalizedPeople,
				Queried: true,
			}
			log.Printf("[DEBUG] Updated connections for %s with people: %v", normalizedPlace, normalizedPeople)

		case "reason":
			connections.ReasoningLog = append(connections.ReasoningLog,
				fmt.Sprintf("Step %d: Analysis - %s",
					steps+1, decision.Reasoning))
			log.Printf("[DEBUG] Processing reasoning step: %s", decision.Reasoning)

		case "answer":
			connections.ReasoningLog = append(connections.ReasoningLog,
				fmt.Sprintf("Step %d: Attempting answer %s - %s",
					steps+1, normalizeString(decision.Answer), decision.Reasoning))
			if decision.Answer == "" {
				log.Printf("[WARN] Empty answer received")
				continue
			}

			answer := normalizeString(decision.Answer)
			if connections.WrongGuesses[answer] {
				log.Printf("[DEBUG] Skipping already tried and incorrect answer: %s", answer)
				continue
			}

			log.Printf("[DEBUG] Attempting answer with normalized city name: %s -> %s", decision.Answer, answer)

			reportRequest := map[string]interface{}{
				"task":   "loop",
				"apikey": centralaAPIKey,
				"answer": answer,
			}

			log.Printf("[DEBUG] Sending report request to %s/report", centralaBaseURL)
			reportURL := fmt.Sprintf("%s/report", centralaBaseURL)
			reportResponse, err := services.PostJSON(reportURL, reportRequest)
			if err != nil {
				log.Printf("[WARN] Failed to send report: %v", err)
				continue
			}
			log.Printf("[DEBUG] Received report response: %s", reportResponse)

			if strings.Contains(reportResponse, "FLG:") {
				log.Printf("[DEBUG] Success! Found flag in response: %s", reportResponse)
				foundFlag = true
				ctx.JSON(http.StatusOK, gin.H{
					"note":           noteContent,
					"connections":    connections,
					"answer":         answer,
					"reportResponse": reportResponse,
				})
				return
			}

			connections.WrongGuesses[answer] = true
			log.Printf("[INFO] Incorrect answer: %s", answer)
		}
	}

	if !foundFlag {
		log.Printf("[ERROR] Investigation failed after %d steps", maxSteps)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find Barbara's location after maximum steps"})
		return
	}
}

func downloadNote(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download note: %w", err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read note content: %w", err)
	}

	// Save to /tmp
	err = os.WriteFile("/tmp/barbara.txt", content, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to save note to /tmp: %w", err)
	}

	return string(content), nil
}

func formatConnectionsToYAML(m map[string]ConnectionInfo) string {
	var result strings.Builder
	for key, info := range m {
		result.WriteString(fmt.Sprintf("    %s:\n", key))
		result.WriteString(fmt.Sprintf("      queried: %v\n", info.Queried))
		result.WriteString("      links:\n")
		for _, value := range info.Links {
			result.WriteString(fmt.Sprintf("      - %s\n", value))
		}
	}
	return result.String()
}

func formatQueriedToYAML(m map[string]bool) string {
	var result strings.Builder
	for key := range m {
		result.WriteString(fmt.Sprintf("      - %s\n", key))
	}
	return result.String()
}

// Helper function to add to unqueried list if not already known
func addUnqueriedEntity(entity string, existing map[string]ConnectionInfo, unqueried *[]string) {
	if _, exists := existing[entity]; !exists {
		// Check if it's not already in unqueried list
		for _, e := range *unqueried {
			if e == entity {
				return
			}
		}
		*unqueried = append(*unqueried, entity)
	}
}

// Helper function to remove from unqueried list
func removeFromUnqueried(entity string, unqueried *[]string) {
	for i, e := range *unqueried {
		if e == entity {
			*unqueried = append((*unqueried)[:i], (*unqueried)[i+1:]...)
			return
		}
	}
}

func formatArrayToYAML(arr []string) string {
	var result strings.Builder
	for _, value := range arr {
		result.WriteString(fmt.Sprintf("      - %s\n", value))
	}
	return result.String()
}
