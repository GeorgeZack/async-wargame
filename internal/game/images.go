package game

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Vignette belongs to one situation brief (turn 0 is the opening briefing).
// Drafts, prompts, and provider diagnostics are private to the referee.
type Vignette struct {
	ID        string    `json:"id"`
	Operation int       `json:"operation"`
	Turn      int       `json:"turn"`
	Prompt    string    `json:"prompt"`
	Caption   string    `json:"caption"`
	Status    string    `json:"status"`
	Approved  bool      `json:"approved,omitempty"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Queued    time.Time `json:"queued"`
	File      string    `json:"file,omitempty"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func (d *Vignette) Busy() bool { return d != nil && (d.Status == RunPending || d.Status == RunRunning) }

func (r *Room) vignetteSlot(op, turn int) **Vignette {
	if turn == 0 && op == max(r.Operation, 1) {
		return &r.BriefingVignette
	}
	if rep := r.report(op, turn); rep != nil {
		return &rep.Vignette
	}
	return nil
}

func (r *Room) Vignette(op, turn int) *Vignette {
	if slot := r.vignetteSlot(op, turn); slot != nil {
		return *slot
	}
	return nil
}

func (r *Room) Vignettes() []*Vignette {
	var out []*Vignette
	if r.BriefingVignette != nil {
		out = append(out, r.BriefingVignette)
	}
	for _, rep := range r.Reports {
		if rep.Vignette != nil {
			out = append(out, rep.Vignette)
		}
	}
	return out
}

func (r *Room) QueueVignette(actor *Member, op, turn int, prompt, caption, provider, model string, now time.Time) error {
	if actor == nil || actor.Role != RoleReferee || actor.Status != StatusApproved {
		return ErrForbidden
	}
	if r.Closed {
		return bad("this room is closed")
	}
	slot := r.vignetteSlot(op, turn)
	if slot == nil {
		return bad("that situation brief no longer exists")
	}
	for _, d := range r.Vignettes() {
		if d.Busy() {
			return bad("an image is already queued or generating in this room; cancel it first")
		}
	}
	prompt, caption = strings.TrimSpace(prompt), strings.TrimSpace(caption)
	if !utf8.ValidString(prompt) || utf8.RuneCountInString(prompt) < 10 || utf8.RuneCountInString(prompt) > 4000 {
		return bad("describe the vignette in 10 to 4,000 characters")
	}
	if !utf8.ValidString(caption) || caption == "" || utf8.RuneCountInString(caption) > 240 {
		return bad("add a public image description of up to 240 characters")
	}
	*slot = &Vignette{ID: newID(), Operation: op, Turn: turn, Prompt: prompt, Caption: caption, Provider: provider, Model: model, Status: RunPending, Queued: now.UTC()}
	return nil
}

func (r *Room) DecideVignette(actor *Member, op, turn int, id, action string) error {
	if actor == nil || actor.Role != RoleReferee || actor.Status != StatusApproved {
		return ErrForbidden
	}
	if r.Closed {
		return bad("this room is closed")
	}
	slot := r.vignetteSlot(op, turn)
	if slot == nil || *slot == nil || (*slot).ID != id {
		return bad("this vignette has been replaced; refresh and try again")
	}
	d := *slot
	switch action {
	case "cancel":
		if !d.Busy() {
			return bad("this image is no longer generating")
		}
		d.Status, d.Error = RunCancelled, "Cancelled. The service may already have charged for the request."
	case "discard":
		*slot = nil
	case "approve":
		if d.Status != RunDone || d.File == "" {
			return bad("wait for an image preview before approving it")
		}
		d.Approved = true
	case "hide":
		d.Approved = false
	default:
		return ErrBadRequest
	}
	return nil
}

// VignettePrompt adds era-neutral illustration guidance to the exact text
// reviewed by the referee. No hidden game context is appended.
func VignettePrompt(description string) string {
	return "Create one evocative vignette illustrating a military situation brief. Depict a single coherent scene, not a map or collage. Follow the era, technology, people, and terrain described; do not assume a historical or futuristic setting. No text, captions, UI, or borders.\n\nSituation to illustrate:\n" + description
}

type ImageAIView struct {
	Available       bool
	Provider, Model string
}

type VignetteView struct {
	Operation, Turn int
	Image           *Vignette
	SuggestedPrompt string
}

func (r *Room) vignetteView(op, turn int, ref bool) VignetteView {
	v := VignetteView{Operation: op, Turn: turn}
	if d := r.Vignette(op, turn); d != nil && (ref || d.Approved && d.Status == RunDone) {
		copy := *d
		if !ref {
			copy.Prompt, copy.Provider, copy.Model, copy.Error = "", "", "", ""
		}
		v.Image = &copy
	}
	if !ref {
		return v
	}
	if v.Image != nil {
		v.SuggestedPrompt = v.Image.Prompt
		return v
	}
	// Only shared prose is suggested. Hidden/holder-only outcomes, orders,
	// rolls, private briefs, and referee notes never enter the prompt.
	var parts []string
	if turn == 0 {
		parts = append(parts, r.Briefing...)
		parts = append(parts, r.Objectives...)
	} else if rep := r.report(op, turn); rep != nil {
		if rep.NarrativeStatus == NarrativeApproved {
			parts = append(parts, rep.Narrative)
		}
		for _, e := range rep.Entries {
			if e.Seen == SeenByAll {
				parts = append(parts, fmt.Sprintf("%s: %s", e.Title, e.Text))
			}
		}
	}
	v.SuggestedPrompt = clip(strings.Join(parts, "\n\n"), 4000)
	return v
}
