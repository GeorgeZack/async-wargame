package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The referee's assistant ("Jev"). It reads the locked orders and suggests
// contests (who rolls what, with which advantages, and why); once the dice
// are rolled, it drafts what happened; after publishing, it drafts an
// opening narrative from what players were told. Everything it writes is a
// suggestion: contests sit unrolled until the referee rolls them, outcome
// drafts until the referee keeps or rewrites them, and narratives until
// the referee approves them for players. It never changes the game.
//
// Model work runs in the background (see internal/assist): game code queues
// a run here, the worker builds the prompt from the room, calls the model
// without holding any lock, and hands the reply back to apply.

// Run kinds.
const (
	RunSuggest    = "suggest"    // contests for every locked order
	RunOutcome    = "outcome"    // what happened, for one order
	RunNarrative  = "narrative"  // the report's opening
	RunIntentions = "intentions" // what the enemy means to do this turn
	RunQuestion   = "question"   // a question to a player about an order
)

// Run statuses.
const (
	RunPending = "pending"
	RunRunning = "running"
	RunDone    = "done"
	RunFailed  = "failed"
)

// Assistant is a room's assistant settings and work.
type Assistant struct {
	On        bool   `json:"on,omitempty"`        // the model may be used in this room
	Suggest   bool   `json:"suggest,omitempty"`   // suggest contests when a turn locks
	Narrative bool   `json:"narrative,omitempty"` // draft a narrative when a turn is published
	Runs      []*Run `json:"runs,omitempty"`
}

// Run is one piece of model work.
type Run struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Target    string    `json:"target,omitempty"` // the order, for an outcome
	Operation int       `json:"operation"`
	Turn      int       `json:"turn"`
	Status    string    `json:"status"`
	Model     string    `json:"model,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Error     string    `json:"error,omitempty"`
	Tokens    int       `json:"tokens,omitempty"`
	Queued    time.Time `json:"queued"`
	Started   time.Time `json:"started,omitzero"`
	Finished  time.Time `json:"finished,omitzero"`
	Prompt    string    `json:"prompt,omitempty"` // prompt version, for the record
}

// Prompt versions, recorded on each run so results can be compared later.
var promptVersions = map[string]string{RunSuggest: "suggest/2", RunOutcome: "outcome/2", RunNarrative: "narrative/1",
	RunIntentions: "intentions/1", RunQuestion: "question/1"}

func (r *Room) queueRun(kind, target string, turn int, now time.Time) {
	op := max(r.Operation, 1)
	r.Assistant.Runs = slices.DeleteFunc(r.Assistant.Runs, func(x *Run) bool {
		return x.Kind == kind && x.Target == target && x.Turn == turn && x.Operation == op && (x.Status == RunPending || x.Status == RunFailed)
	})
	r.Assistant.Runs = append(r.Assistant.Runs, &Run{ID: newID(), Kind: kind, Target: target, Operation: op, Turn: turn, Status: RunPending, Queued: now.UTC()})
	if n := len(r.Assistant.Runs); n > 60 {
		r.Assistant.Runs = r.Assistant.Runs[n-60:]
	}
}

func (r *Room) assistAtLock(now time.Time) {
	if r.Assistant.On && r.Assistant.Suggest && len(r.LockedOrders()) > 0 {
		r.queueRun(RunSuggest, "", r.Turn, now)
	}
}

func (r *Room) assistAtPublish(now time.Time) {
	if r.Assistant.On && r.Assistant.Narrative {
		r.queueRun(RunNarrative, "", r.Turn, now)
	}
}

// --- What the model sees ---

type promptUnit struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Side     string   `json:"side"`
	Count    int      `json:"count,omitempty"`
	Where    string   `json:"where"`
	Bonus    string   `json:"usualBonus"`
	Kit      string   `json:"kit,omitempty"`
	Status   string   `json:"status,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Facts    []string `json:"facts,omitempty"`
	Hidden   bool     `json:"hiddenFromPlayers,omitempty"`
	Intends  string   `json:"intends,omitempty"` // enemy units: the referee's intention for them this turn
	Distance int      `json:"hexesAway,omitempty"`
}

func (r *Room) where(mod *Module, col, row int) string {
	s := fmt.Sprintf("%s (%s, %s)", HexID(col, row), r.Map.SectorAt(col, row), mod.TerrainAt(r.Map.TerrainCode(col, row)).Name)
	for _, f := range r.Features {
		if f.Col == col && f.Row == row {
			s += ", at " + f.Name
		}
	}
	return s
}

func (r *Room) promptUnit(mod *Module, u *Unit) promptUnit {
	cl := mod.Class(u.Class)
	p := promptUnit{ID: u.ID, Name: u.Name, Kind: cl.Name, Side: mod.Side(u.Side).Name, Where: r.where(mod, u.Col, u.Row),
		Bonus: bonusText(mod, u), Kit: KitOf(mod, u), Status: u.Status, Notes: u.Notes, Facts: r.acceptedFacts(u.ID),
		Hidden: !r.VisibleToPlayers(mod, u)}
	if cl.IsGroup() || u.Strength > 1 {
		p.Count = u.Strength
	}
	if !mod.Side(u.Side).Playable {
		p.Intends = r.IntentionOf(u.ID)
	}
	return p
}

func (r *Room) nearby(mod *Module, u *Unit, within int) []promptUnit {
	var out []promptUnit
	for _, x := range r.Units {
		if x == u {
			continue
		}
		if d := HexDistance(u.Col, u.Row, x.Col, x.Row); d <= within {
			p := r.promptUnit(mod, x)
			p.Distance = d
			out = append(out, p)
		}
	}
	return out
}

func (r *Room) settingText(mod *Module) string {
	var fs []string
	for _, f := range mod.Factors {
		fs = append(fs, fmt.Sprintf("%s %+d", f.Name, f.Value))
	}
	return fmt.Sprintf(`The game: %s. %s Each map hex is %s; each turn is %s.
Rolls: when a situation is in doubt, each side rolls a d10 and adds one bonus: everything that matters summed into a single number (how good the unit is, veterancy, the situation, surprise, the weather). The result is just the two totals, like "Okafor 5, the horde 9". There's no rule for what totals mean: the referee compares them, considers what each side was trying to do, and decides. A side can also roll alone, against the situation.
Each unit's usual bonus (quality plus veterancy) is given. Factors that often count: %s.`, mod.Name, mod.Intro, mod.Scale.Hex, mod.Scale.Turn, strings.Join(fs, ", "))
}

// PromptFor builds a run's prompts. It reports false if there's nothing to do.
func (r *Room) PromptFor(mod *Module, run *Run) (system, user string, ok bool) {
	switch run.Kind {
	case RunSuggest:
		return r.suggestPrompt(mod, run)
	case RunOutcome:
		return r.outcomePrompt(mod, run)
	case RunNarrative:
		return r.narrativePrompt(mod, run)
	case RunIntentions:
		return r.intentionsPrompt(mod, run)
	case RunQuestion:
		return r.questionPrompt(mod, run)
	}
	return "", "", false
}

func (r *Room) suggestPrompt(mod *Module, run *Run) (string, string, bool) {
	if run.Turn != r.Turn || r.Phase != PhaseResolving {
		return "", "", false
	}
	type item struct {
		Order  string       `json:"order"`
		Text   string       `json:"orderText"`
		Mark   string       `json:"markedHex,omitempty"`
		Unit   promptUnit   `json:"unit"`
		Nearby []promptUnit `json:"nearby,omitempty"`
	}
	var items []item
	for _, o := range r.LockedOrders() {
		u := r.Unit(o.Unit)
		if u == nil {
			continue
		}
		it := item{Order: o.ID, Text: o.Text, Unit: r.promptUnit(mod, u), Nearby: r.nearby(mod, u, 3)}
		if o.Mark != nil {
			it.Mark = r.where(mod, o.Mark[0], o.Mark[1])
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return "", "", false
	}
	system := `You are Jev, assistant to the referee of a referee-led, old-school roleplaying wargame. Players write free-text orders for their characters; the referee decides what happens, calling for dice only where the outcome is genuinely in doubt and the stakes matter.

` + r.settingText(mod) + `

For each order, suggest the rolls the referee might make, in the order they'd happen. Most orders need zero to two. Don't roll for things that would simply work (walking down an empty street) or simply fail. For each side, list the situational factors that change its usual bonus, with a value and a short reason grounded in the order or the situation: a good plan in the order deserves +1; an ambush +3; darkness -1. Don't repeat the unit's usual bonus; it's added for you. For an opponent that isn't a unit on the list (a locked door, a frightened survivor), give a name and its whole bonus as factors. Mark a roll secret when players shouldn't see it (a hidden enemy noticing them, say). Enemy units may say what they intend this turn ("intends"); the referee decided that before reading the orders, so use it. Use only the unit ids given.

Reply with only a JSON object:
{"contests":[{"order":"<order id>","title":"<short label>","a":{"unit":"<unit id>","factors":[{"name":"<why>","value":<number>}]},"b":{"unit":"<unit id, or empty>","name":"<who, if not a unit>","factors":[{"name":"<why>","value":<number>}]},"secret":false}]}
Leave "b" out when a side rolls alone.`
	b, _ := json.MarshalIndent(map[string]any{"turn": run.Turn, "threat": r.Threat, "facts": r.acceptedFacts(""), "orders": items}, "", " ")
	return system, string(b), true
}

func (r *Room) outcomePrompt(mod *Module, run *Run) (string, string, bool) {
	o := r.lockedOrder(run.Target)
	if o == nil || r.Desk == nil {
		return "", "", false
	}
	u := r.Unit(o.Unit)
	if u == nil {
		return "", "", false
	}
	var rolls []string
	for _, c := range r.Desk.Contests {
		if c.Order == o.ID && c.Result != nil {
			line := c.Title + ": " + c.Result.Text()
			if c.A.Why != "" {
				line += fmt.Sprintf(" (%s's advantage: %s)", c.A.Name, c.A.Why)
			}
			if c.B != nil && c.B.Why != "" {
				line += fmt.Sprintf(" (%s's advantage: %s)", c.B.Name, c.B.Why)
			}
			rolls = append(rolls, line)
		}
	}
	system := `You are Jev, assistant to the referee of a referee-led, old-school roleplaying wargame. The referee has rolled the dice for one player's order. Draft what happened, for the referee to edit before players read it.

` + r.settingText(mod) + `

Write 40 to 130 words, past tense, plain and concrete, like a tough after-action report, about the player's marine by name. Read the rolls as the referee would: compare each side's total with the other's and with what each was trying to do. A big gap is decisive; a small one is close and costs something; a low total rolled alone means the situation got the better of them. Where nothing was rolled, what the order tried simply happens as far as the situation allows. Enemy units nearby may say what they intend this turn; they act on it. Don't invent new units, deaths, or facts that change the scenario; don't reveal hidden enemies the marine couldn't perceive, beyond what they'd see or hear. End with where the marine is and what state they're in if that changed. No headings or lists.`
	b, _ := json.MarshalIndent(map[string]any{
		"order": o.Text, "unit": r.promptUnit(mod, u), "rolls": rolls, "nearby": r.nearby(mod, u, 2),
		"facts": r.acceptedFacts(""), "refereeNotes": r.RefereeNotes}, "", " ")
	return system, string(b), true
}

func (r *Room) narrativePrompt(mod *Module, run *Run) (string, string, bool) {
	rep := r.report(run.Operation, run.Turn)
	if rep == nil {
		return "", "", false
	}
	var lines []string
	for _, e := range rep.Entries {
		if e.Seen == SeenByAll && e.Text != "" {
			lines = append(lines, e.Title+": "+e.Text)
		}
	}
	if len(lines) == 0 {
		return "", "", false
	}
	system := fmt.Sprintf(`You write the short situation report that opens each turn's report in a wargame (%s). Each turn is %s.
Write 60 to 120 words in the voice of a terse radio operator summing up the turn for the squad. Use only the facts in the lines you're given: don't add units, casualties, places, or anything the squad wasn't told. Plain prose; no headings, lists, or sign-off.`, mod.Name, mod.Scale.Turn)
	return system, fmt.Sprintf("Turn %d:\n- %s", rep.Turn, strings.Join(lines, "\n- ")), true
}

// --- Applying replies ---

// ApplyReply turns the model's reply into suggestions or drafts.
func (r *Room) ApplyReply(mod *Module, run *Run, reply string, decode func(string, any) error) error {
	switch run.Kind {
	case RunSuggest:
		return r.applySuggestions(mod, run, reply, decode)
	case RunOutcome:
		return r.applyOutcomeDraft(run, reply)
	case RunNarrative:
		return r.applyNarrative(run, reply)
	case RunIntentions:
		return r.applyIntentions(mod, run, reply, decode)
	case RunQuestion:
		return r.applyQuestion(run, reply)
	}
	return nil
}

func (r *Room) applySuggestions(mod *Module, run *Run, reply string, decode func(string, any) error) error {
	if r.Desk == nil || r.Desk.Turn != run.Turn || r.Phase != PhaseResolving {
		return fmt.Errorf("turn %d is no longer being adjudicated", run.Turn)
	}
	type factor struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	type side struct {
		Unit    string   `json:"unit"`
		Name    string   `json:"name"`
		Factors []factor `json:"factors"`
	}
	var out struct {
		Contests []struct {
			Order  string `json:"order"`
			Title  string `json:"title"`
			A      side   `json:"a"`
			B      *side  `json:"b"`
			Secret bool   `json:"secret"`
		} `json:"contests"`
	}
	if err := decode(reply, &out); err != nil {
		return err
	}
	toParty := func(s side) (PartyInput, bool) {
		in := PartyInput{Unit: s.Unit, Name: s.Name}
		if r.Unit(s.Unit) == nil {
			in.Unit = ""
		}
		var why []string
		sum := 0
		for _, f := range s.Factors {
			v := min(10, max(-10, f.Value))
			sum += v
			if n := strings.TrimSpace(f.Name); n != "" {
				why = append(why, fmt.Sprintf("%s %+d", n, v))
			}
		}
		in.Why = strings.Join(why, ", ")
		if in.Unit != "" {
			in.Situation = sum
		} else {
			in.Bonus = &sum
		}
		return in, in.Unit != "" || in.Name != ""
	}
	added := 0
	for _, c := range out.Contests {
		if added >= 20 || r.lockedOrder(c.Order) == nil {
			continue
		}
		a, ok := toParty(c.A)
		if !ok || r.Unit(a.Unit) == nil {
			continue
		}
		var b *PartyInput
		if c.B != nil {
			if pb, ok := toParty(*c.B); ok {
				b = &pb
			}
		}
		ref := &Member{Role: RoleReferee}
		ct, err := r.AddContest(ref, mod, c.Order, c.Title, a, b, c.Secret)
		if err != nil {
			continue
		}
		ct.Source, ct.Model = "ai", run.Model
		added++
	}
	return nil
}

func (r *Room) applyOutcomeDraft(run *Run, reply string) error {
	if r.Desk == nil || r.Desk.Turn != run.Turn || r.Phase != PhaseResolving {
		return fmt.Errorf("turn %d is no longer being adjudicated", run.Turn)
	}
	o := r.lockedOrder(run.Target)
	if o == nil {
		return fmt.Errorf("that order is gone")
	}
	text := clip(strings.Trim(strings.TrimSpace(reply), "`"), 3000)
	if text == "" {
		return fmt.Errorf("the model returned nothing")
	}
	oc := r.Desk.outcomeFor(o.ID)
	if oc != nil && oc.Text != "" && (oc.Source != "ai" || oc.Edited) {
		// The referee has written their own; keep it, and offer the draft beside it.
		oc.Draft = text
		return nil
	}
	seen := SeenByAll
	if u := r.Unit(o.Unit); u != nil && u.OutOfContact {
		seen = SeenByHolder
	}
	if oc == nil {
		oc = &Outcome{ID: newID(), Order: o.ID}
		r.Desk.Outcomes = append(r.Desk.Outcomes, oc)
	}
	if oc.Seen != "" {
		seen = oc.Seen
	}
	oc.Text, oc.Seen, oc.Source, oc.Model, oc.Edited, oc.Draft = text, seen, "ai", run.Model, false, ""
	return nil
}

func (r *Room) applyNarrative(run *Run, reply string) error {
	rep := r.report(run.Operation, run.Turn)
	if rep == nil {
		return fmt.Errorf("the report for turn %d is gone", run.Turn)
	}
	text := strings.TrimSpace(strings.Trim(strings.TrimSpace(reply), "`"))
	if text == "" {
		return fmt.Errorf("the model returned no narrative")
	}
	rep.Narrative, rep.NarrativeStatus, rep.NarrativeModel = clip(text, 1500), NarrativeDraft, run.Model
	return nil
}

func (r *Room) report(op, turn int) *TurnReport {
	for _, rep := range r.Reports {
		if rep.Turn == turn && max(rep.Operation, 1) == op {
			return rep
		}
	}
	return nil
}

// Narrative statuses.
const (
	NarrativeDraft    = "draft"
	NarrativeApproved = "approved"
)

// --- Worker hooks ---

// NextRun marks the oldest pending run in the room as running and returns
// a copy, or nil.
func (r *Room) NextRun(model, provider string, now time.Time) *Run {
	if !r.Assistant.On {
		return nil
	}
	for _, run := range r.Assistant.Runs {
		if run.Status == RunPending {
			run.Status, run.Started, run.Model, run.Provider, run.Error = RunRunning, now.UTC(), model, provider, ""
			run.Prompt = promptVersions[run.Kind]
			c := *run
			return &c
		}
	}
	return nil
}

// HasPendingRun reports whether the worker has anything to do here.
func (r *Room) HasPendingRun() bool {
	return r.Assistant.On && slices.ContainsFunc(r.Assistant.Runs, func(x *Run) bool { return x.Status == RunPending })
}

// FinishRun records a run's result. apply turns the reply into changes;
// quiet means there was nothing to do, so nobody is told.
func (r *Room) FinishRun(id string, tokens int, runErr error, apply func(*Run) error, quiet bool, now time.Time) {
	i := slices.IndexFunc(r.Assistant.Runs, func(x *Run) bool { return x.ID == id })
	if i < 0 {
		return
	}
	run := r.Assistant.Runs[i]
	run.Finished, run.Tokens = now.UTC(), tokens
	if runErr == nil && apply != nil {
		runErr = apply(run)
	}
	ref := r.Referee()
	if runErr != nil {
		run.Status, run.Error = RunFailed, clip(runErr.Error(), 300)
		if ref != nil {
			r.notify(ref.ID, "The assistant couldn't finish", fmt.Sprintf("%s for turn %d failed: %s", runLabel(run.Kind), run.Turn, run.Error), "turn")
		}
		return
	}
	run.Status = RunDone
	if ref == nil || quiet {
		return
	}
	switch run.Kind {
	case RunSuggest:
		n := 0
		if r.Desk != nil {
			for _, c := range r.Desk.Contests {
				if c.Source == "ai" && c.Result == nil {
					n++
				}
			}
		}
		r.notify(ref.ID, fmt.Sprintf("Turn %d: rolls suggested", run.Turn), fmt.Sprintf("The assistant suggested %s. Check them, then roll.", plural(n, "roll", "rolls")), "turn")
	case RunNarrative:
		r.notify(ref.ID, fmt.Sprintf("Turn %d narrative drafted", run.Turn), "Read it, edit it if you like, and approve it for players.", "reports")
	}
}

// ResetRunning puts runs interrupted by a restart back in the queue.
func (r *Room) ResetRunning() bool {
	changed := false
	for _, run := range r.Assistant.Runs {
		if run.Status == RunRunning {
			run.Status, changed = RunPending, true
		}
	}
	return changed
}

func runLabel(kind string) string {
	return map[string]string{RunSuggest: "Suggesting rolls", RunOutcome: "Drafting an outcome", RunNarrative: "Drafting the narrative",
		RunIntentions: "Proposing intentions", RunQuestion: "Drafting a question"}[kind]
}

// --- Referee actions ---

// SetAssistant changes the room's assistant settings.
func (r *Room) SetAssistant(actor *Member, on, suggest, narrative bool) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	was := r.Assistant.On
	r.Assistant.On, r.Assistant.Suggest, r.Assistant.Narrative = on, suggest, narrative
	if on != was {
		r.logKind(VisReferee, "", "AI assistant switched %s.", map[bool]string{true: "on", false: "off"}[on])
	}
	return nil
}

// RunNow queues assistant work on the referee's request: suggestions for
// the turn being adjudicated, an outcome draft for one order (or every
// order with rolls and no outcome yet, if target is "all"), or a narrative
// for the last report.
func (r *Room) RunNow(actor *Member, kind, target string, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if !r.Assistant.On {
		return bad("switch the AI assistant on for this room first (Room screen)")
	}
	switch kind {
	case RunSuggest:
		if r.Phase != PhaseResolving {
			return bad("rolls can be suggested once the turn is locked")
		}
		r.queueRun(RunSuggest, "", r.Turn, now)
	case RunOutcome:
		if r.Phase != PhaseResolving || r.Desk == nil {
			return bad("outcomes can be drafted once the turn is locked")
		}
		if target != "all" {
			if r.lockedOrder(target) == nil {
				return ErrNotFound
			}
			r.queueRun(RunOutcome, target, r.Turn, now)
			return nil
		}
		for _, o := range r.LockedOrders() {
			if oc := r.Desk.outcomeFor(o.ID); oc == nil || oc.Text == "" {
				r.queueRun(RunOutcome, o.ID, r.Turn, now)
			}
		}
	case RunIntentions:
		if err := r.intentionsOpen(); err != nil {
			return err
		}
		r.queueRun(RunIntentions, "", r.Turn, now)
	case RunQuestion:
		if r.Phase != PhaseResolving || r.lockedOrder(target) == nil {
			return bad("questions are drafted about a locked order")
		}
		r.queueRun(RunQuestion, target, r.Turn, now)
	case RunNarrative:
		if len(r.Reports) == 0 {
			return bad("there's no report to write a narrative for yet")
		}
		r.queueRun(RunNarrative, "", r.Reports[len(r.Reports)-1].Turn, now)
	default:
		return bad("unknown assistant task")
	}
	return nil
}

// UseDraft swaps the assistant's draft in for the referee's outcome text.
func (r *Room) UseDraft(actor *Member, orderID string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return err
	}
	oc := d.outcomeFor(orderID)
	if oc == nil || oc.Draft == "" {
		return ErrNotFound
	}
	oc.Text, oc.Draft, oc.Source, oc.Edited = oc.Draft, "", "ai", false
	return nil
}

// DecideNarrative is the referee approving, editing, or discarding a draft.
func (r *Room) DecideNarrative(actor *Member, op, turn int, action, text string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	rep := r.report(op, turn)
	if rep == nil || rep.Narrative == "" && action != "save" && action != "approve" {
		return ErrNotFound
	}
	switch action {
	case "approve", "save":
		if t := strings.TrimSpace(text); t != "" {
			if t != rep.Narrative && rep.NarrativeModel != "" {
				rep.NarrativeEdited = true
			}
			rep.Narrative = clip(t, 1500)
		}
		if rep.Narrative == "" {
			return bad("write something first")
		}
		if action == "approve" {
			rep.NarrativeStatus = NarrativeApproved
			r.logf(VisAll, "The turn %d report now opens with a narrative.", turn)
		} else if rep.NarrativeStatus == "" {
			rep.NarrativeStatus = NarrativeDraft
		}
	case "withdraw":
		rep.NarrativeStatus = NarrativeDraft
	case "discard":
		rep.Narrative, rep.NarrativeStatus, rep.NarrativeModel, rep.NarrativeEdited = "", "", "", false
	default:
		return bad("approve, save, withdraw, or discard")
	}
	return nil
}

// AIView is the assistant as the referee sees it.
type AIView struct {
	Available bool   // the server has a model
	Provider  string // where game data goes
	Model     string
	On        bool
	Suggest   bool
	Narrative bool
	Runs      []Run // latest first
	Busy      map[string]bool
}

func assistantView(r *Room) AIView {
	a := r.Assistant
	v := AIView{On: a.On, Suggest: a.Suggest, Narrative: a.Narrative, Busy: map[string]bool{}}
	for i := len(a.Runs) - 1; i >= 0; i-- {
		run := a.Runs[i]
		if len(v.Runs) < 6 {
			v.Runs = append(v.Runs, *run)
		}
		if run.Status == RunPending || run.Status == RunRunning {
			v.Busy[run.Kind+"/"+run.Target] = true
		}
	}
	return v
}
