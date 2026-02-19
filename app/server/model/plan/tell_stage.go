package plan

import (
	"log"
	"plandex-server/db"
	shared "plandex-shared"

	"github.com/sashabaranov/go-openai"
)

func (state *activeTellStreamState) lastSuccessfulConvoMessage() *db.ConvoMessage {
	for i := len(state.convo) - 1; i >= 0; i-- {
		msg := state.convo[i]
		if msg.Stopped || msg.Flags.HasError {
			continue
		}
		return msg
	}
	return nil
}

func (state *activeTellStreamState) resolveCurrentStage() (activatePaths map[string]bool, activatePathsOrdered []string) {
	req := state.req
	iteration := state.iteration
	hasContextMap := state.hasContextMap
	convo := state.convo

	log.Printf("[resolveCurrentStage] Initial state: hasContextMap: %v, convo len: %d", hasContextMap, len(convo))

	lastConvoMsg := state.lastSuccessfulConvoMessage()

	activatePaths = map[string]bool{}
	activatePathsOrdered = []string{}

	isContinueFromAssistantMsg := false

	if lastConvoMsg != nil {
		isContinueFromAssistantMsg = iteration == 0 && req.IsUserContinue && lastConvoMsg.Role == openai.ChatMessageRoleAssistant
		log.Printf("[resolveCurrentStage] isContinueFromAssistantMsg: %v (IsUserContinue: %v, LastMsgRole: %s)",
			isContinueFromAssistantMsg, req.IsUserContinue, lastConvoMsg.Role)
	} else {
		log.Println("[resolveCurrentStage] No previous successful conversation message found")
	}

	isUserPrompt := false

	if !isContinueFromAssistantMsg {
		isUserPrompt = lastConvoMsg == nil || lastConvoMsg.Role == openai.ChatMessageRoleUser
		log.Printf("[resolveCurrentStage] isUserPrompt: %v", isUserPrompt)
	}

	var tellStage shared.TellStage
	var planningPhase shared.PlanningPhase

	if req.IsChatOnly {
		tellStage = shared.TellStageChat
		planningPhase = shared.PlanningPhaseTasks
		log.Println("[resolveCurrentStage] Set tellStage to Chat")
	} else if lastConvoMsg == nil || lastConvoMsg.Role == openai.ChatMessageRoleUser || isContinueFromAssistantMsg {
		// Determine stage based on what happened before
		if lastConvoMsg == nil {
			tellStage = shared.TellStagePlanningContext
			planningPhase = shared.PlanningPhaseContext
			log.Println("[resolveCurrentStage] Set tellStage to PlanningContext (initial)")
		} else {
			prevStage := lastConvoMsg.Flags.CurrentStage
			if prevStage.TellStage == shared.TellStageChat {
				tellStage = shared.TellStagePlanningContext
				planningPhase = shared.PlanningPhaseContext
				log.Println("[resolveCurrentStage] Set tellStage to PlanningContext (after Chat)")
			} else if prevStage.TellStage == shared.TellStagePlanningContext {
				tellStage = shared.TellStageDetailedPlanning
				planningPhase = shared.PlanningPhaseDetailed
				log.Println("[resolveCurrentStage] Set tellStage to DetailedPlanning (after PlanningContext)")
			} else if prevStage.TellStage == shared.TellStageDetailedPlanning {
				tellStage = shared.TellStageDetailedPlanning
				planningPhase = shared.PlanningPhaseDetailed
				log.Println("[resolveCurrentStage] Set tellStage to DetailedPlanning (continued)")
			} else {
				tellStage = shared.TellStagePlanningContext
				planningPhase = shared.PlanningPhaseContext
				log.Println("[resolveCurrentStage] Set tellStage to PlanningContext (default)")
			}
		}
	} else {
		// Assistant just replied, move forward
		prevStage := lastConvoMsg.Flags.CurrentStage
		if prevStage.TellStage == shared.TellStagePlanningContext {
			tellStage = shared.TellStageDetailedPlanning
			planningPhase = shared.PlanningPhaseDetailed
			log.Println("[resolveCurrentStage] Set tellStage to DetailedPlanning (assistant replied to PlanningContext)")
		} else if prevStage.TellStage == shared.TellStageChat {
			tellStage = shared.TellStageChat
			planningPhase = shared.PlanningPhaseTasks
			log.Println("[resolveCurrentStage] Stayed in Chat stage")
		} else {
			tellStage = prevStage.TellStage
			planningPhase = prevStage.PlanningPhase
			log.Printf("[resolveCurrentStage] Stayed in stage: %s, phase: %s", tellStage, planningPhase)
		}
	}

	if tellStage == shared.TellStagePlanningContext {
		if lastConvoMsg != nil && lastConvoMsg.Flags.CurrentStage.PlanningPhase == shared.PlanningPhaseContext {
			activatePaths = lastConvoMsg.ActivatedPaths
			activatePathsOrdered = lastConvoMsg.ActivatedPathsOrdered
			log.Printf("[resolveCurrentStage] Copied activatePaths from previous Context phase: %v", activatePaths)
		}
	}

	state.currentStage = shared.CurrentStage{
		TellStage:     tellStage,
		PlanningPhase: planningPhase,
	}
	log.Printf("[resolveCurrentStage] Final state - TellStage: %s, PlanningPhase: %s", tellStage, planningPhase)

	return activatePaths, activatePathsOrdered
}
