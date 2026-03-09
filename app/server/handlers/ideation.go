package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"plandex-server/db"
	"plandex-server/model"
	"plandex-server/model/prompts"
	"plandex-server/types"
	"time"
	"github.com/sashabaranov/go-openai"
)

type IdeationRequest struct {
	Prompt       string            `json:"prompt"`
	AuthVars     map[string]string `json:"authVars"`
	CallResearch bool              `json:"callResearch"`
}

func IdeationStreamHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for IdeationStreamHandler")

	auth := Authenticate(w, r, true)
	if auth == nil {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v\n", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var req IdeationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("Error parsing request body: %v\n", err)
		http.Error(w, "Error parsing request body", http.StatusBadRequest)
		return
	}

	// Fetch INSTALL.md context
	installMd, err := fetchInstallMd()
	if err != nil {
		log.Printf("Error fetching INSTALL.md: %v\n", err)
		installMd = "Guidelines not available."
	}

	// Prepare system prompt
	sysPrompt := prompts.GetIdeationPrompt(installMd)

	// Initialize model client
	settings, _ := db.GetOrgDefaultSettings(auth.OrgId)
	orgUserConfig, _ := db.GetOrgUserConfig(auth.User.Id, auth.OrgId)

	clients := model.InitClients(req.AuthVars, settings, orgUserConfig)

	// Use alias-large (Planner role)
	plannerConfig := settings.GetModelPack().Planner
	modelConfig := plannerConfig.ModelRoleConfig
	baseModelConfig := modelConfig.GetBaseModelConfig(req.AuthVars, settings, orgUserConfig)

	modelReq := types.ExtendedChatCompletionRequest{
		Model: baseModelConfig.ModelName,
		Messages: []types.ExtendedChatMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: []types.ExtendedChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: sysPrompt}},
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: []types.ExtendedChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: req.Prompt}},
			},
		},
		Stream: true,
	}

	stream, err := model.CreateChatCompletionStream(clients, req.AuthVars, &modelConfig, settings, orgUserConfig, auth.OrgId, auth.User.Id, r.Context(), modelReq)
	if err != nil {
		log.Printf("Error starting ideation stream: %v\n", err)
		http.Error(w, "Error starting ideation stream", http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Stream error: %v\n", err)
			break
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			if content != "" {
				fmt.Fprintf(w, "data: %s\n\n", content)
				flusher.Flush()
			}
		}
	}

	if req.CallResearch {
		fmt.Fprintf(w, "data: \n\n### Initiating Research Validation...\n\n")
		flusher.Flush()

		findings, err := runIdeationResearch(req.Prompt, req.AuthVars)
		if err != nil {
			fmt.Fprintf(w, "data: Error during research: %s\n\n", err.Error())
		} else {
			fmt.Fprintf(w, "data: \n\n### Mentor Feedback (Agent Zero Analysis)\n\n%s\n\n", findings)
		}
		flusher.Flush()
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func runIdeationResearch(context string, authVars map[string]string) (string, error) {
	state := &dummyResearchState{authVars: authVars}

	report, err := state.callCriticalCodeAgent(context)
	if err != nil {
		return "", err
	}

	mentorPrompt := fmt.Sprintf("Analyze the following Critical Code report based on the ideation context: '%s'. Provide mentor feedback on the architecture and tasks. \n\nREPORT: %s", context, report)

	return state.callAgentZero(mentorPrompt, "research agent")
}

type dummyResearchState struct {
	authVars map[string]string
}

func (s *dummyResearchState) callCriticalCodeAgent(desc string) (string, error) {
	reqBody := map[string]string{"project_description": desc}
	jsonBody, _ := json.Marshal(reqBody)
	resp, err := http.Post("https://auxteam-critical-code-agent.hf.space/analyze", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var analyzeRes map[string]string
	json.NewDecoder(resp.Body).Decode(&analyzeRes)
	taskId := analyzeRes["task_id"]

	return s.pollReport(taskId)
}

func (s *dummyResearchState) pollReport(taskId string) (string, error) {
	for i := 0; i < 10; i++ {
		time.Sleep(5 * time.Second)
		reportResp, err := http.Get(fmt.Sprintf("https://auxteam-critical-code-agent.hf.space/report/%s", taskId))
		if err == nil && reportResp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(reportResp.Body)
			reportResp.Body.Close()
			return string(body), nil
		}
	}
	return "", fmt.Errorf("Critical Code research timed out")
}

func (s *dummyResearchState) callAgentZero(message string, subagent string) (string, error) {
	reqBody := map[string]string{"message": message, "subagent": subagent}
	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://auxteam-agent-skillset.hf.space/chat", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	token := os.Getenv("AUTHENTICATION_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var azRes map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&azRes)
	if msg, ok := azRes["message"].(string); ok {
		return msg, nil
	}
	return "No feedback received.", nil
}

func fetchInstallMd() (string, error) {
	url := "https://raw.githubusercontent.com/obra/superpowers/refs/heads/main/.opencode/INSTALL.md"
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
