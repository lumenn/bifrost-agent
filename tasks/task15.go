package tasks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"
	"github.com/lumenn/bifrost-agent/services"
)

type Questions struct {
	First  string `json:"01"`
	Second string `json:"02"`
	Third  string `json:"03"`
}

type SolveTask15LLMResponse struct {
	Reasoning       string `json:"reasoning"`
	ExecutionPlan   string `json:"execution_plan"`
	PossibleAnswers any    `json:"possible_answers"`
	Tool            string `json:"tool"`
	Parameters      any    `json:"parameters"`
}

type WebPageMap struct {
	Url     string
	Links   []string
	Visited bool
}

type PossibleAnswer struct {
	Answer    string `json:"answer"`
	Reasoning string `json:"reasoning"`
}

func extractAHrefURLs(text interface{}) []string {
	if text == nil {
		return nil
	}
	textStr, ok := text.(string)
	if !ok {
		return nil
	}
	re := regexp.MustCompile(`<a href="([^"]+)"`)
	return re.FindAllString(textStr, -1)
}

func SolveTask15(ctx *gin.Context, llmService services.LLMService, centralaBaseURL string, centralaAPIKey string, softoBaseURL string) {
	openAIService := llmService.(*services.OpenAiService)
	centralaService := services.NewCentralaService(centralaBaseURL, centralaAPIKey, openAIService)

	body, err := services.GetRequestBody(centralaBaseURL + "/data/" + centralaAPIKey + "/softo.json")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var questions Questions
	err = json.Unmarshal([]byte(body), &questions)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	llmService.SetSystemPrompt(`
	You'r job is to answer questions from the user, to do it, you can use the following tools:
	- FETCH - downloads content of the given URL, and saves it locally - you'll receive path to the saved file.
		Parameters:
			- url: string - URL to download
	- ANSWER - to be used when you have answer to the questions.
		Parameters:
			- answer: string[] - answers to the questions

	Always return the answer in JSON format, like this:
	{
		"reasoning": "explain why specific tool should be used",
		"execution_plan": "explain the steps you'll take to answer the question",
		"tool": "tool_name",
		"possible_answers": {
			"01": {
				"answer": "answer to the first question",
				"reasoning": "explain why this answer is correct"
			},
			"02": {
				"answer": "answer to the second question",
				"reasoning": "explain why this answer is correct"
			},
			"03": {
				"answer": "answer to the third question",
				"reasoning": "explain why this answer is correct"
			}
		},
		"parameters": {
			"parameter_name": "parameter_value",
			"parameter_name2": 2,
			"parameter_name3": ["value1", "value2", "value3"],
			"answer": {
				"01": "answer to the first question",
				"02": "answer to the second question",
				"03": "answer to the third question"	
			}
		}
	}
	
	RULES
	- Answer only based on data available in current message, do not use your knowledge to answer the question.
	- If you need to use a tool, use it.
	- Use TOOLS until you will have answers to all the questions.
	- NEVER return MARKDOWN formatted response, only JSON.
	- NEVER USE MARKDOWN CODE BLOCKS in your response like '''json'''.
	`)

	fetchResult := ""
	extractURLsResult := ""
	answerResult := ""
	actionsTaken := []SolveTask15LLMResponse{}
	webPageMap := []WebPageMap{}
	for {
		tmpl := `
		Please answer the following questions based on the data available in the following page: {{.BaseURL}}
		Ignore any HTML comments.
		Questions:
		- {{.Q1}}
		- {{.Q2}} 
		- {{.Q3}}
		{{if .FetchResult}}
		fetchResult: {{.FetchResult}}
		{{end}}
		{{if .ExtractURLsResult}}
		extractURLsResult: {{.ExtractURLsResult}}
		{{end}}
		{{if .AnswerResult}}
		answerResult: {{.AnswerResult}}
		{{end}}

		{{if .ActionsTaken}}
		Actions taken:
		{{range $index, $action := .ActionsTaken}}
		Tool: {{.Tool}}
		- Reasoning: {{.Reasoning}}
		- Possible Answers:
		{{ if index .PossibleAnswers "01" }}
			- 01: {{index .PossibleAnswers "01"}}
		{{ end }}
		{{ if index .PossibleAnswers "02" }}
			- 02: {{index .PossibleAnswers "02"}}
		{{ end }}
		{{ if index .PossibleAnswers "03" }}
			- 03: {{index .PossibleAnswers "03"}}
		{{ end }}
		- Execution Plan: {{.ExecutionPlan}}
		- Parameters: {{.Parameters}}
		{{end}}
		{{end}}

		{{ if .WebPageMap }}
		Web Page Map:
		{{ range $index, $page := .WebPageMap }}
		URL: {{ $page.Url }}
		- Links: {{ $page.Links }}
		- Visited: {{ $page.Visited }}
		{{ end }}
		{{ end }}
		`

		t := template.Must(template.New("prompt").Parse(tmpl))
		var promptBuf bytes.Buffer

		data := struct {
			BaseURL           string
			Q1, Q2, Q3        string
			FetchResult       string
			ExtractURLsResult string
			AnswerResult      string
			ActionsTaken      []SolveTask15LLMResponse
			WebPageMap        []WebPageMap
		}{
			BaseURL:           softoBaseURL,
			Q1:                questions.First,
			Q2:                questions.Second,
			Q3:                questions.Third,
			FetchResult:       fetchResult,
			ExtractURLsResult: extractURLsResult,
			AnswerResult:      answerResult,
			ActionsTaken:      actionsTaken,
			WebPageMap:        webPageMap,
		}

		if err := t.Execute(&promptBuf, data); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		answer, err := llmService.SendChatMessage(promptBuf.String())

		fetchResult = ""
		extractURLsResult = ""
		answerResult = ""

		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var llmResponse SolveTask15LLMResponse
		err = json.Unmarshal([]byte(answer), &llmResponse)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		switch llmResponse.Tool {
		case "FETCH":
			params, ok := llmResponse.Parameters.(map[string]interface{})
			if !ok {
				continue
			}

			urlStr := params["url"].(string)
			urlExists := false
			for _, page := range webPageMap {
				if page.Url == urlStr {
					urlExists = true
					break
				}
			}
			if urlExists {
				continue
			}

			body, err := services.GetRequestBody(urlStr)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			urls := extractAHrefURLs(body)
			extractURLsResult = strings.Join(urls, ", ")
			webPageMap = append(webPageMap, WebPageMap{
				Url:     urlStr,
				Links:   urls,
				Visited: true,
			})
			fetchResult = body
			actionsTaken = append(actionsTaken, llmResponse)
		case "ANSWER":
			params, ok := llmResponse.Parameters.(map[string]interface{})
			if !ok {
				continue
			}
			response, err := centralaService.PostReport("softo", params["answer"])
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			actionsTaken = append(actionsTaken, llmResponse)

			if strings.Contains(response.Message, "{{FLG:") {
				ctx.JSON(http.StatusOK, gin.H{"body": response})
			} else {
				answerResult = response.Message
			}
		}

	}

	ctx.JSON(http.StatusOK, gin.H{"body": body})
}
