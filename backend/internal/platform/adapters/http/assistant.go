package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	menudomain "aivo/internal/domain/menu"
	"aivo/internal/domain/platform"
	"aivo/internal/platform/app"

	"uuid"
)

// Admin AI assistant: chat per restaurant, model PROPOSES actions,
// nothing applies without the explicit apply endpoint (AGENTS.md AI
// rules; every proposal and applied set is slog-logged).

const (
	maxAssistantFiles   = 8
	maxTextAttachment   = 64 << 10 // 64KB inlined into the prompt
	assistantHistoryLen = 20
)

type assistantMessageView struct {
	ID           uuid.UUID                `json:"id"`
	Role         string                   `json:"role"`
	Text         string                   `json:"text"`
	Attachments  []domain.Attachment      `json:"attachments"`
	Actions      []domain.AssistantAction `json:"actions"`
	ActionStatus *string                  `json:"action_status"`
	CreatedAt    time.Time                `json:"created_at"`
}

func toAssistantMessageView(m domain.AssistantMessage) assistantMessageView {
	if m.Attachments == nil {
		m.Attachments = []domain.Attachment{}
	}
	if m.Actions == nil {
		m.Actions = []domain.AssistantAction{}
	}
	return assistantMessageView{
		ID: m.ID, Role: string(m.Role), Text: m.Text, Attachments: m.Attachments,
		Actions: m.Actions, ActionStatus: (*string)(m.ActionStatus), CreatedAt: m.CreatedAt,
	}
}

// GET /api/v1/restaurants/{id}/assistant/messages?limit=50
func (h *handler) assistantHistory(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	msgs, err := h.AssistantStore.AssistantMessages(r.Context(), rest.ID, limit)
	if writeAppErr(w, err) {
		return
	}
	views := make([]assistantMessageView, len(msgs))
	for i, m := range msgs {
		views[i] = toAssistantMessageView(m)
	}
	writeJSON(w, http.StatusOK, views)
}

// POST /api/v1/restaurants/{id}/assistant/messages — multipart text +
// files[]. Stores the user message, calls the model, stores and returns
// the assistant message. Actions are NOT executed here.
func (h *handler) assistantSend(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	if h.Assistant == nil {
		writeErr(w, http.StatusServiceUnavailable, "assistant_unconfigured", "assistant is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_upload", "multipart body required")
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	files := r.MultipartForm.File["files"]
	if text == "" && len(files) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "text or files required")
		return
	}
	if len(files) > maxAssistantFiles {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", fmt.Sprintf("at most %d files", maxAssistantFiles))
		return
	}

	attachments := []domain.Attachment{}
	textInlines := []string{} // "name:\ncontent" blocks for the prompt
	for _, fh := range files {
		if h.Images == nil {
			writeErr(w, http.StatusServiceUnavailable, "images_unconfigured", "file uploads need image storage configured")
			return
		}
		ct := fh.Header.Get("Content-Type")
		isImage := ct == "image/jpeg" || ct == "image/png" || ct == "image/webp" || ct == "image/gif"
		isText := strings.HasPrefix(ct, "text/") || strings.HasSuffix(fh.Filename, ".md") ||
			strings.HasSuffix(fh.Filename, ".txt") || strings.HasSuffix(fh.Filename, ".csv")
		if !isImage && !isText {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", fmt.Sprintf("unsupported file type %q", ct))
			return
		}
		if isText && fh.Size > maxTextAttachment {
			writeErr(w, http.StatusUnprocessableEntity, "invalid", fmt.Sprintf("%s: text files must be <= 64KB", fh.Filename))
			return
		}

		f, err := fh.Open()
		if err != nil {
			writeAppErr(w, err)
			return
		}
		if isText {
			content, err := io.ReadAll(io.LimitReader(f, maxTextAttachment+1))
			f.Close()
			if err != nil {
				writeAppErr(w, err)
				return
			}
			// Sniff: a "text/*" declaration hiding non-text bytes is
			// rejected; the sniffed type is what gets stored.
			sniffed := http.DetectContentType(content)
			if !strings.HasPrefix(sniffed, "text/") {
				writeErr(w, http.StatusUnprocessableEntity, "invalid", fmt.Sprintf("%s: declared text but content is %s", fh.Filename, sniffed))
				return
			}
			url, err := h.Images.Put(r.Context(), rest.ID, fh.Filename, sniffed, strings.NewReader(string(content)), int64(len(content)))
			if writeAppErr(w, err) {
				return
			}
			attachments = append(attachments, domain.Attachment{Name: fh.Filename, URL: url, Mime: sniffed})
			textInlines = append(textInlines, fh.Filename+":\n"+string(content))
			continue
		}
		reader, sniffed, err := sniffUpload(f, ct)
		if err != nil {
			f.Close()
			writeErr(w, http.StatusUnprocessableEntity, "invalid", fh.Filename+": "+err.Error())
			return
		}
		if !strings.HasPrefix(sniffed, "image/") {
			f.Close()
			writeErr(w, http.StatusUnprocessableEntity, "invalid", fh.Filename+": content is not an image")
			return
		}
		url, err := h.Images.Put(r.Context(), rest.ID, fh.Filename, sniffed, reader, fh.Size)
		f.Close()
		if writeAppErr(w, err) {
			return
		}
		attachments = append(attachments, domain.Attachment{Name: fh.Filename, URL: url, Mime: sniffed})
	}

	threadID, err := h.AssistantStore.AssistantThread(r.Context(), rest.ID)
	if writeAppErr(w, err) {
		return
	}
	userMsg := domain.AssistantMessage{
		ID: uuid.New(), ThreadID: threadID, Role: domain.AssistantRoleUser,
		Text: text, Attachments: attachments,
	}
	if writeAppErr(w, h.AssistantStore.CreateAssistantMessage(r.Context(), rest.ID, userMsg)) {
		return
	}

	prompt, refs, err := h.assistantPrompt(r, rest, text, attachments, textInlines)
	if writeAppErr(w, err) {
		return
	}
	reply, actions, err := h.Assistant.Chat(r.Context(), prompt)
	if writeAppErr(w, err) {
		return
	}

	// Tenant-scope + image-host validation: any bad reference drops the
	// whole action list (reply survives), same rule as the shape check.
	for _, a := range actions {
		if err := domain.ValidateActionRefs(a, refs); err != nil {
			slog.Warn("assistant actions rejected", "restaurant_id", rest.ID, "error", err)
			actions = nil
			break
		}
	}

	asstMsg := domain.AssistantMessage{
		ID: uuid.New(), ThreadID: threadID, Role: domain.AssistantRoleAssistant,
		Text: reply, Actions: actions,
	}
	if writeAppErr(w, h.AssistantStore.CreateAssistantMessage(r.Context(), rest.ID, asstMsg)) {
		return
	}
	actionsJSON, _ := domain.EncodeActions(actions)
	slog.Info("assistant proposal", "restaurant_id", rest.ID, "message_id", asstMsg.ID, "actions", string(actionsJSON))

	asstMsg.CreatedAt = time.Now().UTC()
	writeJSON(w, http.StatusCreated, toAssistantMessageView(asstMsg))
}

// assistantPrompt builds the model prompt (system + state snapshot +
// attachments + recent history + the new request) and the ref sets for
// tenant validation of returned actions.
func (h *handler) assistantPrompt(r *http.Request, rest domain.Restaurant, text string, attachments []domain.Attachment, textInlines []string) (string, domain.ActionRefs, error) {
	menus, err := h.MenuAdmin.Menus(r.Context(), rest.ID)
	if err != nil {
		return "", domain.ActionRefs{}, err
	}
	cats, items, err := h.Menu.Menu(r.Context(), rest.ID)
	if err != nil {
		return "", domain.ActionRefs{}, err
	}
	theme, err := h.Platform.Theme(r.Context(), rest.ID)
	if err != nil {
		return "", domain.ActionRefs{}, err
	}
	history, err := h.AssistantStore.AssistantMessages(r.Context(), rest.ID, assistantHistoryLen)
	if err != nil {
		return "", domain.ActionRefs{}, err
	}

	refs := domain.ActionRefs{
		MenuIDs:     map[uuid.UUID]bool{},
		CategoryIDs: map[uuid.UUID]bool{},
		ItemIDs:     map[uuid.UUID]bool{},
		ImagePrefix: h.ImagePrefix,
	}
	type itemSnap struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		PriceCents int       `json:"price_cents"`
		Available  bool      `json:"available"`
	}
	type catSnap struct {
		ID    uuid.UUID  `json:"id"`
		Name  string     `json:"name"`
		Items []itemSnap `json:"items"`
	}
	type menuSnap struct {
		ID         uuid.UUID `json:"id"`
		Slug       string    `json:"slug"`
		Name       string    `json:"name"`
		IsDefault  bool      `json:"is_default"`
		Categories []catSnap `json:"categories"`
	}
	itemsByCat := map[uuid.UUID][]itemSnap{}
	for _, it := range items {
		refs.ItemIDs[it.ID] = true
		itemsByCat[it.CategoryID] = append(itemsByCat[it.CategoryID], itemSnap{ID: it.ID, Name: it.Name, PriceCents: it.PriceCents, Available: it.Available})
	}
	catsByMenu := map[uuid.UUID][]catSnap{}
	for _, c := range cats {
		refs.CategoryIDs[c.ID] = true
		catsByMenu[c.MenuID] = append(catsByMenu[c.MenuID], catSnap{ID: c.ID, Name: c.Name, Items: itemsByCat[c.ID]})
	}
	snap := []menuSnap{}
	for _, m := range menus {
		refs.MenuIDs[m.ID] = true
		snap = append(snap, menuSnap{ID: m.ID, Slug: m.Slug, Name: m.Name, IsDefault: m.IsDefault, Categories: catsByMenu[m.ID]})
	}
	state, err := json.Marshal(map[string]any{
		"menus": snap,
		"theme": toThemeView(theme, rest.Name),
		"restaurant": map[string]any{
			"name": rest.Name, "slug": rest.Slug, "address": rest.Address, "hours": rest.Hours,
		},
	})
	if err != nil {
		return "", domain.ActionRefs{}, err
	}

	var b strings.Builder
	b.WriteString(`You are the AIVO restaurant admin assistant. Answer the admin's request about their menu/theme and, when the request implies changes, propose them as actions.
Return ONLY a JSON object: {"reply": string (same language as the admin's request), "actions": [Action]}.
Action types (exact fields, ids must come from the current state below):
 create_category{menu_id, name} | rename_category{id, name} | delete_category{id}
 create_item{category_id, name, description, price_cents, allergens[], image_url?} | update_item{id, ...partial} | delete_item{id} | set_item_available{id, available}
 update_theme{theme: {brand_name, accent one of "Blood red"|"Olive"|"Wine"|"Fire", bold, banner_url?, css_vars?}} | create_menu{name, slug}
Money is integer cents. image_url may only be one of the attached image URLs. Propose nothing you are unsure about — say so in the reply instead.

Current state:
`)
	b.Write(state)
	for _, inline := range textInlines {
		b.WriteString("\n\nAttached file ")
		b.WriteString(inline)
	}
	imageListed := false
	for _, a := range attachments {
		if strings.HasPrefix(a.Mime, "image/") {
			if !imageListed {
				b.WriteString("\n\nAttached images (usable as image_url):")
				imageListed = true
			}
			b.WriteString("\n- " + a.Name + ": " + a.URL)
		}
	}
	if len(history) > 0 {
		b.WriteString("\n\nRecent conversation:")
		for _, m := range history {
			b.WriteString("\n" + string(m.Role) + ": " + m.Text)
		}
	}
	b.WriteString("\n\nAdmin request:\n" + text + "\n\nReturn ONLY the JSON object.")
	return b.String(), refs, nil
}

// POST .../assistant/messages/{msgID}/apply {action_indexes?: [int]}
func (h *handler) assistantApply(w http.ResponseWriter, r *http.Request, u domain.User, rest domain.Restaurant) {
	msg, ok := h.assistantDecidableMessage(w, r, rest)
	if !ok {
		return
	}
	var req struct {
		ActionIndexes []int `json:"action_indexes"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	selected := req.ActionIndexes
	if selected == nil {
		for i := range msg.Actions {
			selected = append(selected, i)
		}
	}
	// Validate every index BEFORE executing anything — a bad index means
	// nothing runs.
	if err := validateActionIndexes(selected, len(msg.Actions)); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}

	type result struct {
		Index int    `json:"index"`
		Type  string `json:"type"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	results := []result{}
	// ponytail: sequential stop-on-first-failure, not one transaction —
	// each apply goes through the existing store commands. Batch tx if
	// this ever bites.
	// Tenant refs computed once for the whole batch (~6 queries), and
	// refreshed only after an action that creates a referenceable id.
	refs, err := h.currentRefs(r, rest)
	if writeAppErr(w, err) {
		return
	}
	failed, succeeded := false, 0
	for _, i := range selected {
		a := msg.Actions[i]
		res := result{Index: i, Type: string(a.Type), OK: true}
		if failed {
			res.OK, res.Error = false, "skipped: earlier action failed"
		} else if err := h.applyAction(r, rest, a, refs); err != nil {
			res.OK, res.Error = false, err.Error()
			failed = true
		} else {
			succeeded++
			if a.Type == domain.ActionCreateMenu || a.Type == domain.ActionCreateCategory {
				if refs, err = h.currentRefs(r, rest); err != nil {
					writeAppErr(w, err)
					return
				}
			}
		}
		results = append(results, res)
	}

	// If nothing succeeded, leave the message pending so the admin can
	// retry after fixing the cause; mark applied only on real effect.
	status := domain.ActionStatusApplied
	if succeeded > 0 {
		if writeAppErr(w, h.AssistantStore.SetAssistantMessageStatus(r.Context(), rest.ID, msg.ID, string(domain.ActionStatusApplied))) {
			return
		}
	} else {
		status = "pending"
	}
	actionsJSON, _ := domain.EncodeActions(msg.Actions)
	slog.Info("assistant actions applied", "restaurant_id", rest.ID, "message_id", msg.ID,
		"by_user", u.ID, "selected", fmt.Sprint(selected), "succeeded", succeeded, "actions", string(actionsJSON))
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "results": results})
}

// validateActionIndexes rejects any out-of-range or duplicate index.
func validateActionIndexes(selected []int, n int) error {
	seen := map[int]bool{}
	for _, i := range selected {
		if i < 0 || i >= n {
			return fmt.Errorf("action index %d out of range (0..%d)", i, n-1)
		}
		if seen[i] {
			return fmt.Errorf("action index %d selected twice", i)
		}
		seen[i] = true
	}
	return nil
}

// POST .../assistant/messages/{msgID}/discard
func (h *handler) assistantDiscard(w http.ResponseWriter, r *http.Request, _ domain.User, rest domain.Restaurant) {
	msg, ok := h.assistantDecidableMessage(w, r, rest)
	if !ok {
		return
	}
	if writeAppErr(w, h.AssistantStore.SetAssistantMessageStatus(r.Context(), rest.ID, msg.ID, string(domain.ActionStatusDiscarded))) {
		return
	}
	slog.Info("assistant actions discarded", "restaurant_id", rest.ID, "message_id", msg.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": domain.ActionStatusDiscarded})
}

// assistantDecidableMessage loads {msgID} and checks it's an assistant
// message with pending actions.
func (h *handler) assistantDecidableMessage(w http.ResponseWriter, r *http.Request, rest domain.Restaurant) (domain.AssistantMessage, bool) {
	id, err := uuid.Parse(r.PathValue("msgID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return domain.AssistantMessage{}, false
	}
	msg, err := h.AssistantStore.AssistantMessageByID(r.Context(), rest.ID, id)
	if writeAppErr(w, err) {
		return domain.AssistantMessage{}, false
	}
	if msg.Role != domain.AssistantRoleAssistant || len(msg.Actions) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid", "message has no proposed actions")
		return domain.AssistantMessage{}, false
	}
	if msg.ActionStatus != nil {
		writeErr(w, http.StatusConflict, "conflict", "actions already "+string(*msg.ActionStatus))
		return domain.AssistantMessage{}, false
	}
	return msg, true
}

// applyAction executes one validated action through the existing
// store/app commands. refs is the caller's tenant-ref snapshot (computed
// once per batch; the caller refreshes it after create actions).
func (h *handler) applyAction(r *http.Request, rest domain.Restaurant, a domain.AssistantAction, refs domain.ActionRefs) error {
	ctx := r.Context()
	if err := domain.ValidateAction(a); err != nil {
		return err
	}
	if err := domain.ValidateActionRefs(a, refs); err != nil {
		return err
	}

	switch a.Type {
	case domain.ActionCreateCategory:
		return h.MenuAdmin.CreateCategory(ctx, menudomain.Category{
			ID: uuid.New(), RestaurantID: rest.ID, MenuID: *a.MenuID, Name: strings.TrimSpace(a.Name),
		})
	case domain.ActionRenameCategory:
		cats, _, err := h.Menu.Menu(ctx, rest.ID)
		if err != nil {
			return err
		}
		for _, c := range cats {
			if c.ID == *a.ID {
				c.Name = strings.TrimSpace(a.Name)
				return h.MenuAdmin.UpdateCategory(ctx, c)
			}
		}
		return fmt.Errorf("category not found")
	case domain.ActionDeleteCategory:
		return h.MenuAdmin.DeleteCategory(ctx, rest.ID, *a.ID)
	case domain.ActionCreateItem:
		it := menudomain.MenuItem{
			ID: uuid.New(), RestaurantID: rest.ID, CategoryID: *a.CategoryID,
			Name: strings.TrimSpace(a.Name), PriceCents: *a.PriceCents, Available: true,
		}
		if a.Description != nil {
			it.Description = *a.Description
		}
		if a.ImageURL != nil {
			it.ImageURL = *a.ImageURL
		}
		for _, al := range a.Allergens {
			if !menudomain.ValidAllergen(menudomain.Allergen(al)) {
				return fmt.Errorf("unknown allergen %q", al)
			}
			it.Allergens = append(it.Allergens, menudomain.Allergen(al))
		}
		return h.MenuAdmin.CreateMenuItem(ctx, it)
	case domain.ActionUpdateItem, domain.ActionSetItemAvailable:
		it, err := h.MenuAdmin.MenuItemByID(ctx, rest.ID, *a.ID)
		if err != nil {
			return err
		}
		if a.Name != "" {
			it.Name = strings.TrimSpace(a.Name)
		}
		if a.Description != nil {
			it.Description = *a.Description
		}
		if a.PriceCents != nil {
			it.PriceCents = *a.PriceCents
		}
		if a.ImageURL != nil {
			it.ImageURL = *a.ImageURL
		}
		if a.Allergens != nil {
			it.Allergens = nil
			for _, al := range a.Allergens {
				if !menudomain.ValidAllergen(menudomain.Allergen(al)) {
					return fmt.Errorf("unknown allergen %q", al)
				}
				it.Allergens = append(it.Allergens, menudomain.Allergen(al))
			}
		}
		if a.Available != nil {
			it.Available = *a.Available
		}
		return h.MenuAdmin.UpdateMenuItem(ctx, it)
	case domain.ActionDeleteItem:
		return h.MenuAdmin.DeleteMenuItem(ctx, rest.ID, *a.ID)
	case domain.ActionCreateMenu:
		slug := a.Slug
		if slug == "" {
			slug = app.Slugify(a.Name)
		}
		if !domain.ValidSlug(slug) {
			return fmt.Errorf("invalid menu slug")
		}
		menus, err := h.MenuAdmin.Menus(ctx, rest.ID)
		if err != nil {
			return err
		}
		position := 0
		for _, m := range menus {
			if m.Position >= position {
				position = m.Position + 1
			}
		}
		return h.MenuAdmin.CreateMenu(ctx, menudomain.Menu{
			ID: uuid.New(), RestaurantID: rest.ID, Slug: slug, Name: strings.TrimSpace(a.Name), Position: position,
		})
	case domain.ActionUpdateTheme:
		current, err := h.Platform.Theme(ctx, rest.ID)
		if err != nil {
			return err
		}
		banner := a.Theme.BannerURL
		if banner == "" {
			banner = toThemeView(current, rest.Name).BannerURL
		}
		cssVars := a.Theme.CSSVars
		if cssVars == nil {
			cssVars = map[string]string{}
		}
		themeJSON, err := json.Marshal(map[string]any{
			"brand_name": a.Theme.BrandName, "accent": a.Theme.Accent, "bold": a.Theme.Bold,
			"banner_url": banner, "css_vars": cssVars,
		})
		if err != nil {
			return err
		}
		_, err = h.Platform.SaveTheme(ctx, domain.Theme{RestaurantID: rest.ID, ThemeJSON: themeJSON, DesignMD: current.DesignMD})
		return err
	}
	return fmt.Errorf("unknown action type %q", a.Type)
}

// currentRefs gathers the restaurant's current menu/category/item IDs.
func (h *handler) currentRefs(r *http.Request, rest domain.Restaurant) (domain.ActionRefs, error) {
	refs := domain.ActionRefs{
		MenuIDs:     map[uuid.UUID]bool{},
		CategoryIDs: map[uuid.UUID]bool{},
		ItemIDs:     map[uuid.UUID]bool{},
		ImagePrefix: h.ImagePrefix,
	}
	menus, err := h.MenuAdmin.Menus(r.Context(), rest.ID)
	if err != nil {
		return refs, err
	}
	for _, m := range menus {
		refs.MenuIDs[m.ID] = true
	}
	cats, items, err := h.Menu.Menu(r.Context(), rest.ID)
	if err != nil {
		return refs, err
	}
	for _, c := range cats {
		refs.CategoryIDs[c.ID] = true
	}
	for _, it := range items {
		refs.ItemIDs[it.ID] = true
	}
	return refs, nil
}
