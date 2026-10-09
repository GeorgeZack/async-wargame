package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Enemy intentions. The referee notes what each enemy unit means to do this
// turn ("drift toward the radio noise", "wait under the stairs"), ideally
// while players are still writing orders, so the opposition is decided
// without knowing what the players ordered. Players never see intentions;
// they feed the referee's judgment and the assistant's suggestions and
// drafts. The assistant can propose them from what the enemy would know.

// Intention is what one enemy unit intends this turn.
type Intention struct {
	Turn   int    `json:"turn"`
	Unit   string `json:"unit"`
	Text   string `json:"text"`
	Source string `json:"source,omitempty"` // "ai" if the assistant proposed it
	Model  string `json:"model,omitempty"`
}

const maxIntentionLen = 400

func (r *Room) intention(unitID string) *Intention {
	for _, in := range r.Intentions {
		if in.Turn == r.Turn && in.Unit == unitID {
			return in
		}
	}
	return nil
}

// IntentionOf is a unit's intention this turn, or "".
func (r *Room) IntentionOf(unitID string) string {
	if in := r.intention(unitID); in != nil {
		return in.Text
	}
	return ""
}

func (r *Room) intentionsOpen() error {
	if r.Phase != PhasePlanning && r.Phase != PhaseResolving {
		return bad("intentions are set while a turn is being planned or judged")
	}
	return nil
}

// SetIntention records what an enemy unit intends this turn; empty text
// clears it.
func (r *Room) SetIntention(actor *Member, mod *Module, unitID, text string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if err := r.intentionsOpen(); err != nil {
		return err
	}
	u := r.Unit(unitID)
	if u == nil || u.Destroyed() || mod.Side(u.Side).Playable {
		return bad("intentions are for units the players don't control")
	}
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > maxIntentionLen {
		return bad("keep an intention under %d characters", maxIntentionLen)
	}
	r.Intentions = slices.DeleteFunc(r.Intentions, func(in *Intention) bool { return in.Turn == r.Turn && in.Unit == unitID })
	if text != "" {
		r.Intentions = append(r.Intentions, &Intention{Turn: r.Turn, Unit: unitID, Text: text})
	}
	// Keep only the last few turns.
	r.Intentions = slices.DeleteFunc(r.Intentions, func(in *Intention) bool { return in.Turn < r.Turn-5 })
	return nil
}

// IntentionView is an enemy unit and its intention, for the referee.
type IntentionView struct {
	Unit   UnitView
	Text   string
	Source string
	Model  string
}

func (r *Room) intentionViews(mod *Module, me *Member) []IntentionView {
	var out []IntentionView
	for _, u := range r.Units {
		if s := mod.Side(u.Side); s == nil || !s.Enemy || u.Destroyed() {
			continue
		}
		iv := IntentionView{Unit: unitView(r, u, mod, me)}
		if in := r.intention(u.ID); in != nil {
			iv.Text, iv.Source, iv.Model = in.Text, in.Source, in.Model
		}
		out = append(out, iv)
	}
	return out
}

type noticedUnit struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Side     string `json:"side"`
	Where    string `json:"where"`
	Distance string `json:"distance"`
	Numbers  string `json:"estimatedNumbers"`
}

func (r *Room) intentionsPrompt(mod *Module, run *Run) (string, string, bool) {
	if run.Turn != r.Turn || r.intentionsOpen() != nil {
		return "", "", false
	}
	type enemy struct {
		promptUnit
		Current string        `json:"currentIntention,omitempty"`
		Senses  []noticedUnit `json:"whatItCouldNotice,omitempty"`
	}
	var enemies []enemy
	for _, u := range r.Units {
		if run.Automatic() && !r.autoOwnsUnit(u.ID) {
			continue
		}
		if s := mod.Side(u.Side); s == nil || !s.Enemy || u.Destroyed() {
			continue
		}
		e := enemy{promptUnit: r.promptUnit(mod, u), Current: r.IntentionOf(u.ID)}
		// Do not send the referee's full unit sheets as observed contacts.
		for _, x := range r.Units {
			if r.enemyNotices(mod, u, x) {
				cl := mod.Class(x.Class)
				numbers := "one contact"
				if cl.IsGroup() {
					numbers = estimate(x.Strength)
				}
				e.Senses = append(e.Senses, noticedUnit{x.ID, cl.Name, mod.Side(x.Side).Name, r.where(mod, x.Pos()), r.DistanceText(u.Pos(), x.Pos()), numbers})
			}
		}
		enemies = append(enemies, e)
	}
	if len(enemies) == 0 {
		return "", "", false
	}
	system := `You are Jev, assistant to the referee of a referee-led, old-school roleplaying wargame. Before the turn is judged, the referee decides what the opposition intends, without knowing what the players ordered. Propose one intention for each enemy unit: what it means to do this turn, in one sentence, true to what it is (mindless things follow noise and hunger; cunning things wait and ambush) and based only on its server-filtered whatItCouldNotice contacts. Absent contacts are unknown; never infer their positions, strength or actions. Contact numbers are estimates, not exact headcounts. Keep a unit's current intention if it still makes sense.

` + r.settingText(mod) + `

Reply with only a JSON object:
{"intentions":[{"unit":"<unit id>","intent":"<one sentence>"}]}`
	b, _ := json.MarshalIndent(map[string]any{"turn": run.Turn, "threat": r.Threat, "enemies": enemies}, "", " ")
	return system, string(b), true
}

func (r *Room) applyIntentions(mod *Module, run *Run, reply string, decode func(string, any) error) error {
	if run.Turn != r.Turn {
		return fmt.Errorf("turn %d is over", run.Turn)
	}
	var out struct {
		Intentions []struct {
			Unit   string `json:"unit"`
			Intent string `json:"intent"`
		} `json:"intentions"`
	}
	if err := decode(reply, &out); err != nil {
		return err
	}
	for _, x := range out.Intentions {
		u := r.Unit(x.Unit)
		text := clip(strings.Join(strings.Fields(x.Intent), " "), maxIntentionLen)
		if u == nil || u.Destroyed() || !mod.Side(u.Side).Enemy || text == "" {
			continue
		}
		if cur := r.intention(u.ID); cur != nil && cur.Source != "ai" {
			continue // the referee's own word stands
		}
		r.Intentions = slices.DeleteFunc(r.Intentions, func(in *Intention) bool { return in.Turn == r.Turn && in.Unit == u.ID })
		r.Intentions = append(r.Intentions, &Intention{Turn: r.Turn, Unit: u.ID, Text: text, Source: "ai", Model: run.Model})
	}
	return nil
}

// --- Questions to players ---

// Question is a question about an order: drafted by the referee or the
// assistant, then sent to the player privately.
type Question struct {
	Text   string    `json:"text"`
	Source string    `json:"source,omitempty"` // "ai" for an assistant draft
	Model  string    `json:"model,omitempty"`
	Sent   time.Time `json:"sent,omitzero"`
}

// AskAboutOrder sends a question about a locked order to the player holding
// the unit, as a private message from the referee.
func (r *Room) AskAboutOrder(actor *Member, orderID, text string, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return err
	}
	o := r.lockedOrder(orderID)
	if o == nil {
		return ErrNotFound
	}
	u := r.Unit(o.Unit)
	holder := (*Member)(nil)
	if u != nil {
		holder = r.memberOrNil(u.AssignedTo)
	}
	if holder == nil || holder.Status != StatusApproved {
		return bad("no player holds that unit to ask")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return bad("write the question first")
	}
	if err := r.Post(actor, "ref-"+holder.ID, fmt.Sprintf("About %s's order this turn: %s", u.Name, text), now); err != nil {
		return err
	}
	if d.Questions == nil {
		d.Questions = map[string]*Question{}
	}
	q := d.Questions[orderID]
	if q == nil {
		q = &Question{}
		d.Questions[orderID] = q
	}
	q.Text, q.Sent = text, now.UTC()
	return nil
}

func (r *Room) questionPrompt(mod *Module, run *Run) (string, string, bool) {
	o := r.lockedOrder(run.Target)
	if o == nil || r.Desk == nil {
		return "", "", false
	}
	u := r.Unit(o.Unit)
	if u == nil {
		return "", "", false
	}
	system := `You are Jev, assistant to the referee of a referee-led, old-school roleplaying wargame. A player's order may be read more than one way. Draft one short, friendly question the referee could send the player to settle what they meant, in the referee's voice, addressed to the player. Ask about what matters for how it will go: the route, the sequence, what triggers a fallback, who goes first, what they do if they meet resistance. If the order is perfectly clear, ask about the one detail most likely to matter. One or two sentences, no preamble, no quotation marks.

` + r.settingText(mod)
	b, _ := json.MarshalIndent(map[string]any{"members": r.orderPromptUnits(mod, o), "order": o.Text, "unit": r.promptUnit(mod, u), "nearby": r.nearby(mod, u, 2), "facts": r.acceptedFacts("")}, "", " ")
	return system, string(b), true
}

func (r *Room) applyQuestion(run *Run, reply string) error {
	if r.Desk == nil || r.Desk.Turn != run.Turn || r.Phase != PhaseResolving {
		return fmt.Errorf("turn %d is no longer being judged", run.Turn)
	}
	if r.lockedOrder(run.Target) == nil {
		return fmt.Errorf("that order is gone")
	}
	text := clip(strings.Trim(strings.TrimSpace(reply), "\"`"), 600)
	if text == "" {
		return fmt.Errorf("the model returned nothing")
	}
	if r.Desk.Questions == nil {
		r.Desk.Questions = map[string]*Question{}
	}
	// A fresh draft replaces whatever was there, sent or not.
	r.Desk.Questions[run.Target] = &Question{Text: text, Source: "ai", Model: run.Model}
	return nil
}
