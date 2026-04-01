package pipe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/codeany-ai/codeany/internal/config"
	"github.com/codeany-ai/open-agent-sdk-go/agent"
	"github.com/codeany-ai/open-agent-sdk-go/types"
)

// Run executes a single prompt in non-interactive mode
func Run(ctx context.Context, cfg *config.Config, prompt string, outputFmt string) error {
	cwd, _ := os.Getwd()

	opts := agent.Options{
		Model:              cfg.Model,
		APIKey:             cfg.APIKey,
		BaseURL:            cfg.BaseURL,
		CWD:                cwd,
		MaxTurns:           cfg.MaxTurns,
		MaxBudgetUSD:       cfg.MaxBudgetUSD,
		PermissionMode:     cfg.GetPermissionMode(),
		MCPServers:         cfg.MCPServers,
		SystemPrompt:       cfg.SystemPrompt,
		AppendSystemPrompt: cfg.AppendSystemPrompt,
		CustomHeaders:      cfg.CustomHeaders,
		ProxyURL:           cfg.ProxyURL,
	}

	a := agent.New(opts)
	defer a.Close()

	if err := a.Init(ctx); err != nil {
		return fmt.Errorf("failed to initialize agent: %w", err)
	}

	switch outputFmt {
	case "json":
		return runJSON(ctx, a, prompt)
	case "stream-json":
		return runStreamJSON(ctx, a, prompt)
	default:
		return runText(ctx, a, prompt)
	}
}

func runText(ctx context.Context, a *agent.Agent, prompt string) error {
	events, errCh := a.Query(ctx, prompt)

	for event := range events {
		if event.Type == types.MessageTypeAssistant && event.Message != nil {
			text := types.ExtractText(event.Message)
			if text != "" {
				fmt.Print(text)
			}
		}
	}

	if err := <-errCh; err != nil {
		return err
	}

	fmt.Println()
	return nil
}

func runJSON(ctx context.Context, a *agent.Agent, prompt string) error {
	result, err := a.Prompt(ctx, prompt)
	if err != nil {
		return err
	}

	output := map[string]interface{}{
		"text":      result.Text,
		"numTurns":  result.NumTurns,
		"cost":      result.Cost,
		"duration":  result.Duration.String(),
		"usage":     result.Usage,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func runStreamJSON(ctx context.Context, a *agent.Agent, prompt string) error {
	events, errCh := a.Query(ctx, prompt)
	enc := json.NewEncoder(os.Stdout)

	for event := range events {
		enc.Encode(map[string]interface{}{
			"type":    event.Type,
			"message": event.Message,
			"text":    event.Text,
			"cost":    event.Cost,
		})
	}

	return <-errCh
}
