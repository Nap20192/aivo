package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"
)

// fakeGen returns a Generator whose CLI invocation yields out (as the
// envelope's result field) — no real exec.
func fakeGen(t *testing.T, modelText string) *Generator {
	t.Helper()
	envelope, err := json.Marshal(map[string]string{"result": modelText})
	if err != nil {
		t.Fatal(err)
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) { return envelope, nil }
	return g
}

func current() domain.Theme {
	return domain.Theme{
		ThemeJSON: json.RawMessage(`{"brand_name":"Ember & Bone","accent":"Blood red","bold":false,"banner_url":"http://x/banner.jpg"}`),
		DesignMD:  "# brief",
	}
}

func themeOf(t *testing.T, th domain.Theme) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(th.ThemeJSON, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGenerateValidProposal(t *testing.T) {
	g := fakeGen(t, `{"brand_name":"Ember","accent":"Olive","bold":true,"banner_url":"http://evil/x.png","css_vars":{"--accent-solid":"#556b2f"},"extra_field":"dropped"}`)
	th, err := g.Generate(context.Background(), "# brief", current())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	m := themeOf(t, th)
	if m["accent"] != "Olive" || m["bold"] != true || m["brand_name"] != "Ember" {
		t.Errorf("theme = %v", m)
	}
	// Model can never change the banner; current one is kept.
	if m["banner_url"] != "http://x/banner.jpg" {
		t.Errorf("banner_url = %v, want current kept", m["banner_url"])
	}
	if _, ok := m["extra_field"]; ok {
		t.Error("unknown field survived")
	}
	if th.DesignMD != "# brief" {
		t.Errorf("design_md = %q", th.DesignMD)
	}
}

func TestGenerateStripsFences(t *testing.T) {
	g := fakeGen(t, "```json\n{\"brand_name\":\"E\",\"accent\":\"Wine\",\"bold\":false,\"css_vars\":{}}\n```")
	th, err := g.Generate(context.Background(), "b", current())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if themeOf(t, th)["accent"] != "Wine" {
		t.Error("fenced JSON not parsed")
	}
}

func TestGenerateRejects(t *testing.T) {
	cases := map[string]string{
		"accent outside enum": `{"brand_name":"E","accent":"Hot pink","bold":false}`,
		"bad css var name":    `{"brand_name":"E","accent":"Fire","bold":false,"css_vars":{"background":"red"}}`,
		"css url() value":     `{"brand_name":"E","accent":"Fire","bold":false,"css_vars":{"--x":"url(http://evil)"}}`,
		"css semicolon":       `{"brand_name":"E","accent":"Fire","bold":false,"css_vars":{"--x":"red; position:fixed"}}`,
		"css brace":           `{"brand_name":"E","accent":"Fire","bold":false,"css_vars":{"--x":"red } body {"}}`,
		"css expression()":    `{"brand_name":"E","accent":"Fire","bold":false,"css_vars":{"--x":"Expression(alert(1))"}}`,
		"empty brand":         `{"brand_name":"  ","accent":"Fire","bold":false}`,
		"not json":            `the theme should feel warm`,
	}
	for name, modelText := range cases {
		if _, err := fakeGen(t, modelText).Generate(context.Background(), "b", current()); !errors.Is(err, ports.ErrThemeGeneration) {
			t.Errorf("%s: got %v, want ErrThemeGeneration", name, err)
		}
	}
}

func TestGenerateCLIFailure(t *testing.T) {
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	if _, err := g.Generate(context.Background(), "b", current()); !errors.Is(err, ports.ErrThemeGeneration) {
		t.Errorf("cli failure: got %v, want ErrThemeGeneration", err)
	}
}

func TestPromptCarriesBriefAndSchema(t *testing.T) {
	p := buildPrompt("wood-fire, olive tones", current())
	for _, want := range []string{"wood-fire, olive tones", `"Blood red" | "Olive" | "Wine" | "Fire"`, "Ember & Bone"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
