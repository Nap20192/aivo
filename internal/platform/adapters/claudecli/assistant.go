package claudecli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"
)

const assistantTimeout = 120 * time.Second

// Assistant implements ports.Assistant over the same `claude` CLI as the
// theme generator: strict JSON out, hard validation, typed error or a
// clean result — never half-parsed actions.
type Assistant struct {
	bin string
	run runner
}

var _ ports.Assistant = (*Assistant)(nil)

func NewAssistant(bin string) *Assistant {
	if bin == "" {
		bin = "claude"
	}
	return &Assistant{bin: bin, run: execRun}
}

func (a *Assistant) Chat(ctx context.Context, prompt string) (string, []domain.AssistantAction, error) {
	ctx, cancel := context.WithTimeout(ctx, assistantTimeout)
	defer cancel()

	out, err := a.run(ctx, a.bin, "-p", prompt, "--output-format", "json")
	if err != nil {
		return "", nil, fmt.Errorf("%w: run cli: %v", ports.ErrAssistant, err)
	}
	return parseAssistantOutput(out)
}

type assistantOutput struct {
	Reply   string                   `json:"reply"`
	Actions []domain.AssistantAction `json:"actions"`
}

// parseAssistantOutput unwraps the CLI envelope and validates every
// proposed action's shape. Per the contract, an invalid or unknown
// action rejects the WHOLE action list but keeps the reply.
func parseAssistantOutput(cliOut []byte) (string, []domain.AssistantAction, error) {
	var env cliEnvelope
	if err := json.Unmarshal(cliOut, &env); err != nil {
		return "", nil, fmt.Errorf("%w: cli envelope: %v", ports.ErrAssistant, err)
	}
	if strings.TrimSpace(env.Result) == "" {
		return "", nil, fmt.Errorf("%w: empty result", ports.ErrAssistant)
	}

	var out assistantOutput
	if err := json.Unmarshal([]byte(stripFences(env.Result)), &out); err != nil {
		return "", nil, fmt.Errorf("%w: output json: %v", ports.ErrAssistant, err)
	}
	if strings.TrimSpace(out.Reply) == "" {
		return "", nil, fmt.Errorf("%w: empty reply", ports.ErrAssistant)
	}

	for _, action := range out.Actions {
		if err := domain.ValidateAction(action); err != nil {
			// Keep the reply, drop every action (all-or-nothing per list).
			return out.Reply, nil, nil
		}
	}
	if out.Actions == nil {
		out.Actions = []domain.AssistantAction{}
	}
	return out.Reply, out.Actions, nil
}
