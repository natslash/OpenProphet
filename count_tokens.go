package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"prophet-trader/services"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

// To access unexported function convertGenaiSchema, we'll redefine it here temporarily or just count the prompt without schemas if needed.
// Better: we can instantiate geminiClient, but wait, gemini_client.go doesn't expose a way to count.
func main() {
	godotenv.Load()
	ctx := context.Background()
	apiKey := os.Getenv("GEMINI_API_KEY")
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatal(err)
	}
	modelName := os.Getenv("LLM_MODEL")
	if modelName == "" {
		modelName = "gemini-3.1-pro-preview"
	}
	model := client.GenerativeModel(modelName)

	rulesData, _ := ioutil.ReadFile("TRADING_RULES.md")
	systemPrompt := string(rulesData)
	systemPrompt += "\n\nCRITICAL CONTEXT:\n- Timezone: CET (Central European Time)\n- Base Currency: EUR (€)\nEnsure all price values, portfolio calculations, and temporal reasoning naturally default to Euros and CET without requiring manual prompting."

	var parts []genai.Part
	parts = append(parts, genai.Text(systemPrompt))
	// Add a dummy tool representation to approximate tokens since we can't easily pass tools to CountTokens directly in this old SDK sometimes.
	tools := services.BuildAgentTools()
	for _, t := range tools {
		parts = append(parts, genai.Text(fmt.Sprintf("%s %s", t.Name, t.Description)))
	}

	resp, err := model.CountTokens(ctx, parts...)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Total tokens for prefix: %d\n", resp.TotalTokens)
}
