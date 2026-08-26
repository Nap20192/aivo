// Assistant types: restaurant-scoped chat where the model PROPOSES
// menu/theme actions and nothing applies without explicit confirmation
// (AGENTS.md AI rules). Actions cross a trust boundary — every payload
// is validated by ValidateAction plus the tenant-scope check in
// ValidateActionRefs before it is stored, and again before apply.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aivo/internal/sharedkernel"
)

// AssistantRole is an assistant chat message's author.
type AssistantRole string

// Assistant message roles.
const (
	AssistantRoleUser      AssistantRole = "user"
	AssistantRoleAssistant AssistantRole = "assistant"
)

// Default is the role of a freshly composed message.
func (AssistantRole) Default() AssistantRole { return AssistantRoleUser }

// Valid reports whether r is user or assistant.
func (r AssistantRole) Valid() bool { return r == AssistantRoleUser || r == AssistantRoleAssistant }

// ActionStatus is the admin's decision on a message's proposed actions.
// The zero value ("") is not a named status: it means pending decision,
// tracked in Go via a nil *ActionStatus rather than a member of this
// type.
type ActionStatus string

// Assistant action statuses (nil = pending decision).
const (
	ActionStatusApplied   ActionStatus = "applied"
	ActionStatusDiscarded ActionStatus = "discarded"
)

// Default is the zero value, meaning no decision yet.
func (ActionStatus) Default() ActionStatus { return "" }

// Valid reports whether s is applied or discarded.
func (s ActionStatus) Valid() bool { return s == ActionStatusApplied || s == ActionStatusDiscarded }

// Attachment is one uploaded file on a user message.
type Attachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Mime string `json:"mime"`
}

// ActionType is a proposed assistant action's kind.
type ActionType string

// Allowed assistant action types.
const (
	ActionCreateCategory   ActionType = "create_category"
	ActionRenameCategory   ActionType = "rename_category"
	ActionDeleteCategory   ActionType = "delete_category"
	ActionCreateItem       ActionType = "create_item"
	ActionUpdateItem       ActionType = "update_item"
	ActionDeleteItem       ActionType = "delete_item"
	ActionSetItemAvailable ActionType = "set_item_available"
	ActionUpdateTheme      ActionType = "update_theme"
	ActionCreateMenu       ActionType = "create_menu"
)

// Default is the first-declared action type.
func (ActionType) Default() ActionType { return ActionCreateCategory }

// Valid reports whether t is one of the nine known action types.
func (t ActionType) Valid() bool {
	switch t {
	case ActionCreateCategory, ActionRenameCategory, ActionDeleteCategory, ActionCreateItem,
		ActionUpdateItem, ActionDeleteItem, ActionSetItemAvailable, ActionUpdateTheme, ActionCreateMenu:
		return true
	}
	return false
}

// AssistantAction is one proposed action. One flat struct for all types;
// which fields matter depends on Type (see ValidateAction). Unknown JSON
// fields are dropped on decode.
type AssistantAction struct {
	Type ActionType `json:"type"`

	// References into the restaurant's data.
	ID         *sharedkernel.ID `json:"id,omitempty"`          // rename/delete category, item ops
	MenuID     *sharedkernel.ID `json:"menu_id,omitempty"`     // create_category
	CategoryID *sharedkernel.ID `json:"category_id,omitempty"` // create_item

	// Content fields.
	Name        string   `json:"name,omitempty"`
	Slug        string   `json:"slug,omitempty"` // create_menu
	Description *string  `json:"description,omitempty"`
	PriceCents  *int     `json:"price_cents,omitempty"`
	Allergens   []string `json:"allergens,omitempty"`
	ImageURL    *string  `json:"image_url,omitempty"`
	Available   *bool    `json:"available,omitempty"`

	// update_theme payload (same schema as the theme generator).
	Theme *ThemePayload `json:"theme,omitempty"`
}

// ThemePayload is the theme schema shared with the generator.
type ThemePayload struct {
	BrandName string            `json:"brand_name"`
	Accent    string            `json:"accent"`
	Bold      bool              `json:"bold"`
	BannerURL string            `json:"banner_url,omitempty"`
	CSSVars   map[string]string `json:"css_vars,omitempty"`
}

// ErrInvalidAction marks a rejected action payload.
var ErrInvalidAction = errors.New("invalid assistant action")

// ValidAccent reports whether a is one of the four theme accents.
func ValidAccent(a string) bool {
	switch a {
	case "Blood red", "Olive", "Wine", "Fire":
		return true
	}
	return false
}

var cssVarNameRe = regexp.MustCompile(`^--[a-z0-9-]+$`)

// ValidCSSVar is the CSS injection guard shared by the theme generator
// and the assistant: model-proposed custom properties become inline CSS
// on diner pages, so anything that could break out of a declaration or
// load remote content is rejected.
func ValidCSSVar(name, value string) error {
	if !cssVarNameRe.MatchString(name) {
		return fmt.Errorf("name %q invalid (want --lowercase-name)", name)
	}
	if len(value) == 0 || len(value) > 200 {
		return errors.New("value empty or too long")
	}
	lower := strings.ToLower(value)
	// Backslash rejected outright: CSS escapes ("\75rl(" spells url()
	// character-by-character) have no legitimate use in theme values.
	for _, bad := range []string{"url(", "expression(", ";", "{", "}", `\`} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("value contains %q", bad)
		}
	}
	return nil
}

// ValidateAction checks type allowlist and per-type payload shape
// (everything that needs no DB access). Tenant scoping of referenced IDs
// is ValidateActionRefs.
func ValidateAction(a AssistantAction) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s: %s", ErrInvalidAction, a.Type, fmt.Sprintf(format, args...))
	}
	needName := func() error {
		if strings.TrimSpace(a.Name) == "" {
			return fail("name is required")
		}
		return nil
	}

	switch a.Type {
	case ActionCreateCategory:
		if a.MenuID == nil {
			return fail("menu_id is required")
		}
		return needName()
	case ActionRenameCategory:
		if a.ID == nil {
			return fail("id is required")
		}
		return needName()
	case ActionDeleteCategory, ActionDeleteItem:
		if a.ID == nil {
			return fail("id is required")
		}
		return nil
	case ActionCreateItem:
		if a.CategoryID == nil {
			return fail("category_id is required")
		}
		if err := needName(); err != nil {
			return err
		}
		if a.PriceCents == nil || *a.PriceCents < 0 {
			return fail("price_cents must be an integer >= 0")
		}
		return nil
	case ActionUpdateItem:
		if a.ID == nil {
			return fail("id is required")
		}
		if a.PriceCents != nil && *a.PriceCents < 0 {
			return fail("price_cents must be >= 0")
		}
		return nil
	case ActionSetItemAvailable:
		if a.ID == nil || a.Available == nil {
			return fail("id and available are required")
		}
		return nil
	case ActionCreateMenu:
		if err := needName(); err != nil {
			return err
		}
		if a.Slug != "" && !ValidSlug(a.Slug) {
			return fail("invalid slug")
		}
		return nil
	case ActionUpdateTheme:
		if a.Theme == nil {
			return fail("theme payload is required")
		}
		if !ValidAccent(a.Theme.Accent) {
			return fail("accent %q not in enum", a.Theme.Accent)
		}
		if strings.TrimSpace(a.Theme.BrandName) == "" || len(a.Theme.BrandName) > 100 {
			return fail("brand_name empty or too long")
		}
		if len(a.Theme.CSSVars) > 40 {
			return fail("too many css_vars")
		}
		for name, value := range a.Theme.CSSVars {
			if err := ValidCSSVar(name, value); err != nil {
				return fail("css var %s: %v", name, err)
			}
		}
		return nil
	default:
		return fail("unknown action type")
	}
}

// ActionRefs is the set of IDs that belong to the restaurant, gathered
// by the caller from the store, plus the allowed public image URL
// prefix ("" = no image storage configured, image_url rejected).
type ActionRefs struct {
	MenuIDs     map[sharedkernel.ID]bool
	CategoryIDs map[sharedkernel.ID]bool
	ItemIDs     map[sharedkernel.ID]bool
	ImagePrefix string
}

// ValidateActionRefs checks tenant scoping: every referenced ID must
// belong to this restaurant, and image URLs must live on our public S3
// host. Run after ValidateAction.
func ValidateActionRefs(a AssistantAction, refs ActionRefs) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s: %s", ErrInvalidAction, a.Type, fmt.Sprintf(format, args...))
	}
	if a.MenuID != nil && !refs.MenuIDs[*a.MenuID] {
		return fail("menu_id not in this restaurant")
	}
	if a.CategoryID != nil && !refs.CategoryIDs[*a.CategoryID] {
		return fail("category_id not in this restaurant")
	}
	if a.ID != nil {
		switch a.Type {
		case ActionRenameCategory, ActionDeleteCategory:
			if !refs.CategoryIDs[*a.ID] {
				return fail("category id not in this restaurant")
			}
		case ActionUpdateItem, ActionDeleteItem, ActionSetItemAvailable:
			if !refs.ItemIDs[*a.ID] {
				return fail("item id not in this restaurant")
			}
		}
	}
	if a.ImageURL != nil && *a.ImageURL != "" {
		if refs.ImagePrefix == "" || !strings.HasPrefix(*a.ImageURL, refs.ImagePrefix) {
			return fail("image_url must be on the platform image host")
		}
	}
	return nil
}

// AssistantMessage is one chat message. Actions is non-empty only on
// assistant messages that propose changes; ActionStatus tracks the
// admin's decision.
type AssistantMessage struct {
	ID           sharedkernel.ID
	ThreadID     sharedkernel.ID
	Role         AssistantRole
	Text         string
	Attachments  []Attachment
	Actions      []AssistantAction
	ActionStatus *ActionStatus
	CreatedAt    time.Time
}

// EncodeActions / DecodeActions keep the jsonb round trip in one place.
func EncodeActions(actions []AssistantAction) ([]byte, error) {
	if actions == nil {
		actions = []AssistantAction{}
	}
	return json.Marshal(actions)
}
