package handlers

import (
	"log"
	"net/http"
	"os"
	"plandex-server/db"
	"plandex-server/hooks"
	"plandex-server/model"
	"plandex-server/types"
	shared "plandex-shared"
)

type initClientsParams struct {
	w    http.ResponseWriter
	auth *types.ServerAuth

	apiKeys     map[string]string // deprecated
	openAIOrgId string            // deprecated

	authVars map[string]string

	plan          *db.Plan
	settings      *shared.PlanSettings
	orgUserConfig *shared.OrgUserConfig
}

type initClientsResult struct {
	clients  map[string]model.ClientInfo
	authVars map[string]string
}

func initClients(params initClientsParams) initClientsResult {
	w := params.w
	settings := params.settings
	orgUserConfig := params.orgUserConfig

	authVars := map[string]string{}
	if params.authVars != nil {
		authVars = params.authVars
	} else if params.apiKeys != nil {
		authVars = map[string]string{}
		for envVar, apiKey := range params.apiKeys {
			authVars[envVar] = apiKey
		}
		if params.openAIOrgId != "" {
			authVars["OPENAI_ORG_ID"] = params.openAIOrgId
		}
	}

	hookResult, apiErr := hooks.ExecHook(hooks.GetIntegratedModels, hooks.HookParams{
		Auth: params.auth,
		Plan: params.plan,
	})

	if apiErr != nil {
		log.Printf("Error getting integrated models: %v\n", apiErr)
		http.Error(w, "Error getting integrated models", http.StatusInternalServerError)
		return initClientsResult{}
	}

	if hookResult.GetIntegratedModelsResult != nil && hookResult.GetIntegratedModelsResult.IntegratedModelsMode {
		merged := map[string]string{}
		for k, v := range hookResult.GetIntegratedModelsResult.AuthVars {
			merged[k] = v
		}
		if authVars[shared.AnthropicClaudeMaxTokenEnvVar] != "" {
			merged[shared.AnthropicClaudeMaxTokenEnvVar] = authVars[shared.AnthropicClaudeMaxTokenEnvVar]
		}
		authVars = merged
	}
	// Inject server-side environment variables for known providers if not already set
	for _, envVar := range []string{
		shared.OpenAIEnvVar,
		shared.OpenRouterApiKeyEnvVar,
		shared.AnthropicApiKeyEnvVar,
		shared.GoogleAIStudioApiKeyEnvVar,
		shared.AzureOpenAIEnvVar,
		shared.DeepSeekApiKeyEnvVar,
		shared.PerplexityApiKeyEnvVar,
		shared.BlabladorApiKeyEnvVar,
		"AUTHENTICATION_TOKEN",
		"HELMHOLTZ_API_KEY",
		"SERVER_API_KEY",
	} {
		if authVars[envVar] == "" && os.Getenv(envVar) != "" {
			authVars[envVar] = os.Getenv(envVar)
		}
	}

	// Fallback for Blablador if specifically Helmholtz key is provided
	if authVars[shared.BlabladorApiKeyEnvVar] == "" && authVars["HELMHOLTZ_API_KEY"] != "" {
		authVars[shared.BlabladorApiKeyEnvVar] = authVars["HELMHOLTZ_API_KEY"]
	}

	if len(authVars) == 0 && os.Getenv("IS_CLOUD") != "" {
		log.Println("No api keys/credentials provided for models")
		http.Error(w, "No api keys/credentials provided for models", http.StatusBadRequest)
		return initClientsResult{}
	}

	clients := model.InitClients(authVars, settings, orgUserConfig)

	return initClientsResult{
		clients:  clients,
		authVars: authVars,
	}
}
