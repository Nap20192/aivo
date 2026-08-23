package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aivo/internal/domain/platform"
	"aivo/internal/platform/ports"
)

func fakeAssistant(t *testing.T, modelText string) *Assistant {
	t.Helper()
	envelope, err := json.Marshal(map[string]string{"result": modelText})
	if err != nil {
		t.Fatal(err)
	}
	a := NewAssistant("")
	a.run = func(context.Context, string, ...string) ([]byte, error) { return envelope, nil }
	return a
}

func TestAssistantParsesReplyAndActions(t *testing.T) {
	a := fakeAssistant(t, "```json\n"+`{"reply":"Добавил Цезарь.","actions":[{"type":"create_item","category_id":"6e9f7a52-0000-4000-8000-000000000001","name":"Цезарь","price_cents":1200}]}`+"\n```")
	reply, actions, err := a.Chat(context.Background(), "p")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply != "Добавил Цезарь." || len(actions) != 1 {
		t.Fatalf("reply=%q actions=%d", reply, len(actions))
	}
	if actions[0].Type != domain.ActionCreateItem || *actions[0].PriceCents != 1200 {
		t.Errorf("action = %+v", actions[0])
	}
}

func TestAssistantUnknownActionDropsListKeepsReply(t *testing.T) {
	a := fakeAssistant(t, `{"reply":"ok","actions":[{"type":"drop_database"}]}`)
	reply, actions, err := a.Chat(context.Background(), "p")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply != "ok" || actions != nil {
		t.Errorf("reply=%q actions=%v, want reply kept + nil actions", reply, actions)
	}
}

func TestAssistantBadOutputTypedError(t *testing.T) {
	for name, modelText := range map[string]string{
		"not json":    "sure, here's what I'd do",
		"empty reply": `{"reply":"  "}`,
	} {
		if _, _, err := fakeAssistant(t, modelText).Chat(context.Background(), "p"); !errors.Is(err, ports.ErrAssistant) {
			t.Errorf("%s: got %v, want ErrAssistant", name, err)
		}
	}
}
