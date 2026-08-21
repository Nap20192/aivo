// Package claudecli implements ports.ThemeGenerator by shelling out to
// the `claude` CLI (`claude -p <prompt> --output-format json`). The
// model's output crosses a trust boundary: everything is parsed and
// validated strictly here — accent enum, css_vars name/value guards —
// and any failure returns ports.ErrThemeGeneration, never a half-parsed
// theme.
package claudecli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"aivo/internal/platform/domain"
	"aivo/internal/platform/ports"
)

const timeout = 60 * time.Second

// runner executes the CLI and returns stdout. Swapped in tests.
type runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

type Generator struct {
	bin string
	run runner
}

var _ ports.ThemeGenerator = (*Generator)(nil)

// New builds a Generator using bin (empty = "claude").
func New(bin string) *Generator {
	if bin == "" {
		bin = "claude"
	}
	return &Generator{bin: bin, run: execRun}
}

func execRun(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, firstLine(string(ee.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (g *Generator) Generate(ctx context.Context, designMD string, current domain.Theme) (domain.Theme, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := g.run(ctx, g.bin, "-p", buildPrompt(designMD, current), "--output-format", "json")
	if err != nil {
		return domain.Theme{}, fmt.Errorf("%w: run cli: %v", ports.ErrThemeGeneration, err)
	}
	return parseAndValidate(out, current)
}

func buildPrompt(designMD string, current domain.Theme) string {
	return `You translate a restaurant's design brief into a menu theme JSON.
Allowed schema, return ONLY this JSON object and nothing else:
{
  "brand_name": string,
  "accent": one of exactly "Blood red" | "Olive" | "Wine" | "Fire",
  "bold": boolean,
  "banner_url": string (keep the current value unchanged),
  "css_vars": object mapping CSS custom property names ("--name") to plain CSS values (no url(), no semicolons, no braces)
}
Current theme JSON:
` + string(currentJSON(current)) + `

Design brief:
---
` + designMD + `
---
Return ONLY the theme JSON object.`
}

func currentJSON(t domain.Theme) json.RawMessage {
	if len(t.ThemeJSON) == 0 {
		return json.RawMessage(`{}`)
	}
	return t.ThemeJSON
}

// cliEnvelope is the `--output-format json` wrapper; result holds the
// model's text.
type cliEnvelope struct {
	Result string `json:"result"`
}

// proposedTheme is the only shape accepted from the model; unknown
// fields are dropped by construction.
type proposedTheme struct {
	BrandName string            `json:"brand_name"`
	Accent    string            `json:"accent"`
	Bold      bool              `json:"bold"`
	BannerURL string            `json:"banner_url"`
	CSSVars   map[string]string `json:"css_vars"`
}

var cssVarNameRe = regexp.MustCompile(`^--[a-z0-9-]+$`)

const (
	maxCSSVars     = 40
	maxCSSValueLen = 200
	maxBrandLen    = 100
)

// parseAndValidate turns raw CLI stdout into a validated Theme, keeping
// current's banner_url and design_md. Any problem returns
// ports.ErrThemeGeneration.
func parseAndValidate(cliOut []byte, current domain.Theme) (domain.Theme, error) {
	fail := func(format string, args ...any) (domain.Theme, error) {
		return domain.Theme{}, fmt.Errorf("%w: "+format, append([]any{ports.ErrThemeGeneration}, args...)...)
	}

	var env cliEnvelope
	if err := json.Unmarshal(cliOut, &env); err != nil {
		return fail("cli envelope: %v", err)
	}
	if strings.TrimSpace(env.Result) == "" {
		return fail("empty result")
	}

	var p proposedTheme
	if err := json.Unmarshal([]byte(stripFences(env.Result)), &p); err != nil {
		return fail("theme json: %v", err)
	}

	switch p.Accent {
	case "Blood red", "Olive", "Wine", "Fire":
	default:
		return fail("accent %q not in enum", p.Accent)
	}
	p.BrandName = strings.TrimSpace(p.BrandName)
	if p.BrandName == "" || len(p.BrandName) > maxBrandLen {
		return fail("brand_name empty or too long")
	}
	if len(p.CSSVars) > maxCSSVars {
		return fail("too many css_vars (%d)", len(p.CSSVars))
	}
	if p.CSSVars == nil {
		p.CSSVars = map[string]string{}
	}
	for name, value := range p.CSSVars {
		if !cssVarNameRe.MatchString(name) {
			return fail("css var name %q invalid", name)
		}
		if err := checkCSSValue(value); err != nil {
			return fail("css var %s: %v", name, err)
		}
	}

	// banner_url is never taken from the model — keep the current one.
	themeJSON, err := json.Marshal(map[string]any{
		"brand_name": p.BrandName,
		"accent":     p.Accent,
		"bold":       p.Bold,
		"banner_url": currentBannerURL(current),
		"css_vars":   p.CSSVars,
	})
	if err != nil {
		return fail("encode: %v", err)
	}
	return domain.Theme{
		RestaurantID: current.RestaurantID,
		ThemeJSON:    themeJSON,
		DesignMD:     current.DesignMD,
	}, nil
}

// checkCSSValue is the CSS injection guard: values become inline custom
// properties in diner pages, so anything that could break out of a
// declaration or load remote content is rejected.
func checkCSSValue(v string) error {
	if len(v) == 0 || len(v) > maxCSSValueLen {
		return fmt.Errorf("empty or too long")
	}
	lower := strings.ToLower(v)
	for _, bad := range []string{"url(", "expression(", ";", "{", "}"} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("contains %q", bad)
		}
	}
	return nil
}

func currentBannerURL(t domain.Theme) string {
	var stored struct {
		BannerURL string `json:"banner_url"`
	}
	json.Unmarshal(t.ThemeJSON, &stored)
	return stored.BannerURL
}

// stripFences removes a wrapping ```...``` markdown fence (with optional
// language tag) around the model text, if present.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // drop the "json" (or other) tag line
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
