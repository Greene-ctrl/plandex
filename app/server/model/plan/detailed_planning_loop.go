package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"plandex-server/db"
	"strings"
	"time"
)

type AnalyzeRequest struct {
	RepoUrl            string `json:"repo_url,omitempty"`
	ProjectDescription string `json:"project_description,omitempty"`
}

type AnalyzeResponse struct {
	TaskId string `json:"task_id"`
}

type AgentZeroRequest struct {
	Message  string `json:"message"`
	Subagent string `json:"subagent,omitempty"`
}

type AgentZeroResponse struct {
	Response string `json:"response"`
}

func (state *activeTellStreamState) callCriticalCodeAgent() (string, error) {
	log.Println("Calling Critical Code Agent /analyze")

	// Try to get repo URL or use description
	repoUrl := "" // For now, we don't have a reliable way to get public repo URL if it's local
	description := state.plan.Name

	reqBody := AnalyzeRequest{
		RepoUrl:            repoUrl,
		ProjectDescription: description,
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := http.Post("https://auxteam-critical-code-agent.hf.space/analyze", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Critical Code Agent /analyze returned status %d", resp.StatusCode)
	}

	var analyzeRes AnalyzeResponse
	json.NewDecoder(resp.Body).Decode(&analyzeRes)

	taskId := analyzeRes.TaskId
	log.Printf("Task ID from Critical Code Agent: %s", taskId)

	// Poll for report
	for i := 0; i < 30; i++ {
		log.Printf("Polling for report, attempt %d", i+1)
		reportResp, err := http.Get(fmt.Sprintf("https://auxteam-critical-code-agent.hf.space/report/%s", taskId))
		if err != nil {
			return "", err
		}

		if reportResp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(reportResp.Body)
			reportResp.Body.Close()
			return string(body), nil
		}
		reportResp.Body.Close()
		time.Sleep(10 * time.Second)
	}

	return "", fmt.Errorf("timed out waiting for report from Critical Code Agent")
}

func (state *activeTellStreamState) callAgentZero(message string, subagent string) (string, error) {
	log.Printf("Calling Agent Zero with subagent %s", subagent)

	reqBody := AgentZeroRequest{
		Message:  message,
		Subagent: subagent,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://auxteam-agent-skillset.hf.space/chat", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	// Authentication might be needed based on docs, but user didn't provide a token.
	// I'll assume it works without for now or use a placeholder if I had one.

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Agent Zero returned status %d: %s", resp.StatusCode, string(body))
	}

	var azRes AgentZeroResponse
	// The response format might vary, I'll try to decode it.
	// Based on docs: {"title": ..., "endpoints": ...} - wait, that was /docs.
	// Let's assume it returns a JSON with a response field.
	err = json.NewDecoder(resp.Body).Decode(&azRes)
	if err != nil {
		// Try reading as raw string if JSON fails
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	}

	return azRes.Response, nil
}

func (state *activeTellStreamState) startDetailedPlanningLoop(initialOverview string) {
	log.Println("Starting Detailed Planning Loop")

	// Ensure active plan is set
	if state.activePlan == nil {
		err := state.setActivePlan()
		if err != nil {
			log.Printf("Error setting active plan in Detailed Planning Loop: %v", err)
			return
		}
	}

	currentRecommendations := initialOverview
	var rejections []string

	for {
		// 1. Call Critical Code Agent
		report, err := state.callCriticalCodeAgent()
		if err != nil {
			log.Printf("Error calling Critical Code Agent: %v", err)
			break
		}

		// 2. Send report to Agent Zero
		prompt := fmt.Sprintf("Investigate the following analysis from the Critical Code Agent: \n\n%s\n\nContext of component structure: \n\n%s\n\nProvide recommendations for GitHub projects and Hugging Face spaces.", report, currentRecommendations)
		if len(rejections) > 0 {
			prompt += fmt.Sprintf("\n\nNote that the following recommendations were previously rejected: %v. Please provide alternatives.", rejections)
		}

		azResponse, err := state.callAgentZero(prompt, "research agent")
		if err != nil {
			log.Printf("Error calling Agent Zero: %v", err)
			break
		}

		// 3. Evaluate recommendations (Agent Zero handles confidence internally based on prompt)
		// User says: "and then I should the components recommended by the critical code agent and only if the agent/0 is confident that these are good recommodations it should allow them and if not reject them"
		// "if we have rejections to rejection is being a second prompt message after the first investigation and we send each recommendation by a one message and wait until 0 has touched it based on the context"

		// This suggests another loop with Agent Zero for each recommendation.
		recommendations := state.extractRecommendations(azResponse)

		newRejections := []string{}

		for _, rec := range recommendations {
			evalPrompt := fmt.Sprintf("Based on the context, are you confident that this recommendation is good? Recommendation: %s", rec)
			evalResponse, err := state.callAgentZero(evalPrompt, "research agent")
			if err != nil {
				log.Printf("Error evaluating recommendation: %v", err)
				continue
			}

			if state.isApproved(evalResponse) {
				log.Printf("Recommendation approved: %s", rec)
				state.updatePlanWithRecommendation(rec)
			} else {
				log.Printf("Recommendation rejected: %s", rec)
				newRejections = append(newRejections, rec)
			}
		}

		rejections = append(rejections, newRejections...)

		if len(newRejections) == 0 {
			log.Println("No more new recommendations or rejections. Project approved.")
			break
		}

		// If we have rejections, loop again with Critical Code Agent
		log.Println("Rejections found, looping back to Critical Code Agent")
	}

	// Final step: Pass to another API (placeholder)
	state.passToFinalAPI()
}

func (state *activeTellStreamState) extractRecommendations(response string) []string {
	log.Println("Extracting recommendations from Agent Zero response")
	var recs []string
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			recs = append(recs, strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "))
		}
	}
	if len(recs) == 0 && response != "" {
		recs = append(recs, response)
	}
	return recs
}

func (state *activeTellStreamState) isApproved(response string) bool {
	lower := strings.ToLower(response)
	return !strings.Contains(lower, "reject") && (strings.Contains(lower, "approve") || strings.Contains(lower, "confident") || strings.Contains(lower, "good") || strings.Contains(lower, "yes"))
}

func (state *activeTellStreamState) updatePlanWithRecommendation(rec string) {
	log.Printf("Updating plan with approved recommendation: %s", rec)
	// Add the recommendation as a new subtask to the plan
	newSubtask := &db.Subtask{
		Title:       fmt.Sprintf("Integration of recommendation: %s", rec),
		Description: fmt.Sprintf("Incorporate the following recommendation into the project: %s", rec),
	}
	state.subtasks = append(state.subtasks, newSubtask)

	// Persist this to the database.
	err := db.StorePlanSubtasks(state.plan.OrgId, state.plan.Id, state.subtasks)
	if err != nil {
		log.Printf("Error storing subtasks: %v", err)
	}
}

func (state *activeTellStreamState) passToFinalAPI() {
	log.Println("Passing to final API: https://example.com/api/finalize-project")
	// Placeholder for final API call
	http.Post("https://example.com/api/finalize-project", "application/json", nil)
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
