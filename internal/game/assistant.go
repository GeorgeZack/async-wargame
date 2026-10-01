package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The referee's assistant ("Jev"). It turns a locked turn into a short
// queue of issue cards for the referee, and drafts a narrative for each
// published report. Some checks are plain rules and always run. With an AI
// model configured on the server and switched on for the room, the model
// also reviews every order against the situation, and drafts narratives.
//
// The assistant advises; it never changes the game. Every card says whether
// a rule or the model raised it (and which model), and the referee decides
// what to do with it. Narratives are drafted only from the lines players
// already get, and players see one only after the referee approves it.
//
// Model work runs in the background (see internal/assist): game code queues
// a run here, the worker builds the prompt from the room, calls the model
// without holding any lock, and hands the reply back to apply.

// Run kinds.
const (
	RunOrderReview = "order-review"
	RunNarrative   = "narrative"
)

// Run statuses.
const (
	RunPending = "pending"
	RunRunning = "running"
	RunDone    = "done"
	RunFailed  = "failed"
)

// Issue severities, most urgent first.
const (
	SeverityBlocking = "blocking" // decide before publishing
	SeverityJudgment = "judgment" // needs the referee's eye
	SeverityInfo     = "info"     // worth knowing
)

// Issue statuses.
const (
	IssueOpen      = "open"
	IssueNoted     = "noted"
	IssueDismissed = "dismissed"
	IssueSent      = "sent" // forwarded to the player
)

// Assistant is a room's assistant settings and work.
type Assistant struct {
	On        bool     `json:"on,omitempty"`        // the model may be used in this room
	Review    bool     `json:"review,omitempty"`    // review orders at lock
	Narrative bool     `json:"narrative,omitempty"` // draft report narratives
	Runs      []*Run   `json:"runs,omitempty"`
	Issues    []*Issue `json:"issues,omitempty"`
}

// Run is one piece of model work.
type Run struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
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

// Issue is one card for the referee.
type Issue struct {
	ID         string    `json:"id"`
	Operation  int       `json:"operation"`
	Turn       int       `json:"turn"`
	Unit       string    `json:"unit,omitempty"`
	UnitName   string    `json:"unitName,omitempty"`
	Severity   string    `json:"severity"`
	Text       string    `json:"text"`
	Suggestion string    `json:"suggestion,omitempty"`
	Source     string    `json:"source"` // "rules" or "ai"
	Model      string    `json:"model,omitempty"`
	Status     string    `json:"status"`
	Note       string    `json:"note,omitempty"`
	DecidedAt  time.Time `json:"decidedAt,omitzero"`
}

// Prompt versions, recorded on each run so results can be compared later.
const (
	promptOrderReview = "order-review/1"
	promptNarrative   = "narrative/1"
)

// queueRun adds a run, replacing a pending or failed one of the same kind
// and turn.
func (r *Room) queueRun(kind string, turn int, now time.Time) {
	op := max(r.Operation, 1)
	r.Assistant.Runs = slices.DeleteFunc(r.Assistant.Runs, func(x *Run) bool {
		return x.Kind == kind && x.Turn == turn && x.Operation == op && (x.Status == RunPending || x.Status == RunFailed)
	})
	r.Assistant.Runs = append(r.Assistant.Runs, &Run{ID: newID(), Kind: kind, Operation: op, Turn: turn, Status: RunPending, Queued: now.UTC()})
	if n := len(r.Assistant.Runs); n > 40 {
		r.Assistant.Runs = r.Assistant.Runs[n-40:]
	}
}

// assistAtLock raises rule-based cards and queues the model's review.
func (r *Room) assistAtLock(mod *Module, now time.Time) {
	r.ruleIssues(mod)
	if r.Assistant.On && r.Assistant.Review {
		r.queueRun(RunOrderReview, r.Turn, now)
	}
}

// assistAtPublish queues a narrative for the report just published.
func (r *Room) assistAtPublish(now time.Time) {
	if r.Assistant.On && r.Assistant.Narrative {
		r.queueRun(RunNarrative, r.Turn, now)
	}
}

func (r *Room) addIssue(is *Issue) {
	is.ID, is.Operation, is.Turn, is.Status = newID(), max(r.Operation, 1), r.Turn, IssueOpen
	if u := r.Unit(is.Unit); u != nil {
		is.UnitName = u.Name
	}
	r.Assistant.Issues = append(r.Assistant.Issues, is)
	if n := len(r.Assistant.Issues); n > 200 {
		r.Assistant.Issues = r.Assistant.Issues[n-200:]
	}
}

// ruleIssues are the checks that need no model.
func (r *Room) ruleIssues(mod *Module) {
	for _, o := range r.Orders {
		if o.Turn != r.Turn || o.Status != OrderLocked {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil {
			continue
		}
		t := mod.Orders.Task(o.Task)
		switch {
		case t != nil && t.Offensive && outOfAmmo(mod.Class(u.Class), u):
			r.addIssue(&Issue{Unit: u.ID, Severity: SeverityJudgment, Source: "rules",
				Text:       fmt.Sprintf("%s is ordered to %s with no ammunition left.", u.Name, strings.ToLower(t.Name)),
				Suggestion: "It fights at half strength; consider whether it should even try."})
		case o.Unconfirmed:
			r.addIssue(&Issue{Unit: u.ID, Severity: SeverityInfo, Source: "rules",
				Text: fmt.Sprintf("%s is running an unconfirmed standing order (%s), defensively only.", u.Name, r.orderSummary(mod, o))})
		case t != nil && (t.Destination || t.Offensive) && strings.TrimSpace(o.Intent) == "":
			r.addIssue(&Issue{Unit: u.ID, Severity: SeverityInfo, Source: "rules",
				Text: fmt.Sprintf("%s's order (%s) gives no intent, so there's nothing to judge a surprise against.", u.Name, r.orderSummary(mod, o))})
		}
	}
}

// --- Order review ---

// reviewOrder is one order as the model sees it.
type reviewOrder struct {
	Unit      string   `json:"unit"`
	Name      string   `json:"name"`
	Class     string   `json:"class"`
	Strength  int      `json:"strength"`
	Ammo      string   `json:"ammo,omitempty"`
	Hex       string   `json:"hex"`
	Terrain   string   `json:"terrain"`
	Radio     string   `json:"radio,omitempty"`
	Task      string   `json:"task"`
	TaskNote  string   `json:"taskNote,omitempty"`
	Dest      string   `json:"destination,omitempty"`
	Steps     int      `json:"routeHexes,omitempty"`
	Cost      string   `json:"movementUsed,omitempty"`
	Intent    string   `json:"intent,omitempty"`
	Risk      string   `json:"risk"`
	Fallback  string   `json:"fallback"`
	Standing  string   `json:"standing,omitempty"`
	Outcome   string   `json:"outcome"`
	NearEnemy []string `json:"enemiesWithin4Hexes,omitempty"`
}

// OrderReviewPrompt builds the system and user prompts for a run. It reports
// false if there's nothing to review.
func (r *Room) OrderReviewPrompt(mod *Module, run *Run) (system, user string, ok bool) {
	where := func(c, row int) string {
		return fmt.Sprintf("%s (%s, %s)", HexID(c, row), r.Map.SectorAt(c, row), mod.TerrainAt(r.Map.TerrainCode(c, row)).Name)
	}
	opt := func(list []Option, id string) string {
		for _, o := range list {
			if o.ID == id {
				return o.Name
			}
		}
		return id
	}
	var orders []reviewOrder
	for _, o := range r.Orders {
		if o.Turn != run.Turn || o.Status != OrderLocked {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil || !mod.Side(u.Side).Playable {
			continue
		}
		cl := mod.Class(u.Class)
		ro := reviewOrder{Unit: u.ID, Name: u.Name, Class: cl.Name, Strength: u.Strength, Ammo: ammoText(cl, u),
			Hex: where(o.FromCol, o.FromRow), Terrain: mod.TerrainAt(r.Map.TerrainCode(o.FromCol, o.FromRow)).Name,
			Task: o.Task, Intent: o.Intent, Risk: opt(mod.Orders.Risks, o.Risk), Fallback: opt(mod.Orders.Fallbacks, o.Fallback)}
		if u.OutOfContact {
			ro.Radio = "out of contact"
		}
		if t := mod.Orders.Task(o.Task); t != nil {
			ro.Task, ro.TaskNote = t.Name, t.Note
		}
		if o.HasDest {
			ro.Dest, ro.Steps, ro.Cost = where(o.DestCol, o.DestRow), len(o.Route), fmt.Sprintf("%d of %d", o.Cost, cl.Movement)
		}
		switch {
		case o.Unconfirmed:
			ro.Standing = "standing order nobody confirmed this turn: runs defensively only"
		case o.Standing:
			ro.Standing = "standing order, confirmed"
		}
		ro.Outcome = r.outcomeFor(u, o)
		for _, e := range r.Units {
			if mod.Side(e.Side).Enemy && HexDistance(e.Col, e.Row, o.FromCol, o.FromRow) <= 4 {
				ro.NearEnemy = append(ro.NearEnemy, fmt.Sprintf("%s (%s, %d) at %s", e.Name, mod.Class(e.Class).Name, e.Strength, HexID(e.Col, e.Row)))
			}
		}
		orders = append(orders, ro)
	}
	if len(orders) == 0 {
		return "", "", false
	}
	var tasks []string
	for _, t := range mod.Orders.Tasks {
		tasks = append(tasks, fmt.Sprintf("- %s: %s", t.Name, t.Note))
	}
	system = fmt.Sprintf(`You are Jev, the assistant to the referee of a turn-based wargame (%s, rules %s). Each hex is %s and each turn is %s.
Orders for the turn are locked and the rules engine has already worked out movement and fights. Your job is to read every order against the situation and flag what the referee should look at before publishing. You advise the referee only; you never decide.

Flag things like: an intent the task or destination doesn't serve; a route into or next to known enemies under a cautious risk setting; a fallback that contradicts the plan; offensive orders from weak, out-of-ammo, or out-of-contact units; two orders that conflict or depend on each other; vague intent where the outcome could reasonably be read two ways. Don't repeat what an outcome already plainly says, don't invent units or facts, and don't flag an order that's simply sensible. Saying nothing is fine.

Severity: "blocking" when the referee should decide something before publishing; "judgment" when a ruling could reasonably go either way; "info" when it's merely worth knowing.

Tasks in this game:
%s

Reply with only a JSON object, no prose:
{"issues":[{"unit":"<unit id from the list>","severity":"blocking|judgment|info","issue":"<one or two sentences>","suggestion":"<what the referee might do, one sentence>"}]}`,
		mod.Name, mod.Version, mod.Scale.Hex, mod.Scale.Turn, strings.Join(tasks, "\n"))
	b, _ := json.MarshalIndent(map[string]any{"turn": run.Turn, "threat": r.Threat, "orders": orders}, "", " ")
	return system, string(b), true
}

// outcomeFor says what resolution did with a unit this turn.
func (r *Room) outcomeFor(u *Unit, o *Order) string {
	res := r.Resolution
	if res == nil || res.Turn != o.Turn {
		return "not yet resolved"
	}
	var parts []string
	for _, m := range res.Moves {
		if m.Unit == u.ID {
			reason := map[string]string{"engaged": "stopped by contact", "stacking": "stopped short: the hex was full", "stopped": "stopped short of its goal", "cautious": "stopped short to stay out of contact"}[m.Reason]
			if reason == "" {
				reason = "arrived"
			}
			parts = append(parts, fmt.Sprintf("moved %s → %s, %s", HexID(m.From()[0], m.From()[1]), HexID(m.To()[0], m.To()[1]), reason))
		}
	}
	for _, e := range res.Encounters {
		if slices.Contains(e.Friend, u.ID) {
			f := fmt.Sprintf("in a fight at %s against %d enemy units", HexID(e.Col, e.Row), len(e.Enemy))
			if e.Settled {
				f += fmt.Sprintf(" (%s %s)", strings.TrimSuffix(winnerText(e), "."), e.Outcome.Result)
			}
			parts = append(parts, f)
		}
	}
	if len(parts) == 0 {
		return "held its position"
	}
	return strings.Join(parts, "; ")
}

// ApplyOrderReview turns the model's reply into issue cards.
func (r *Room) ApplyOrderReview(run *Run, reply string, decode func(string, any) error) error {
	var out struct {
		Issues []struct {
			Unit       string `json:"unit"`
			Severity   string `json:"severity"`
			Issue      string `json:"issue"`
			Suggestion string `json:"suggestion"`
		} `json:"issues"`
	}
	if err := decode(reply, &out); err != nil {
		return err
	}
	inTurn := map[string]bool{}
	for _, o := range r.Orders {
		if o.Turn == run.Turn && o.Status == OrderLocked {
			inTurn[o.Unit] = true
		}
	}
	n := 0
	for _, is := range out.Issues {
		if n >= 15 || strings.TrimSpace(is.Issue) == "" {
			continue
		}
		if !slices.Contains([]string{SeverityBlocking, SeverityJudgment, SeverityInfo}, is.Severity) {
			is.Severity = SeverityInfo
		}
		if !inTurn[is.Unit] {
			is.Unit = "" // the model named a unit that has no order: keep the point, drop the link
		}
		r.addIssue(&Issue{Unit: is.Unit, Severity: is.Severity, Source: "ai", Model: run.Model,
			Text: clip(is.Issue, 400), Suggestion: clip(is.Suggestion, 300)})
		n++
	}
	return nil
}

// --- Narrative ---

// NarrativePrompt builds the prompts for a report's narrative, from the
// players' version of the report only.
func (r *Room) NarrativePrompt(mod *Module, run *Run) (system, user string, ok bool) {
	rep := r.report(run.Operation, run.Turn)
	if rep == nil || len(rep.Players) == 0 {
		return "", "", false
	}
	system = fmt.Sprintf(`You write the situation report that opens each turn of a wargame (%s). Each turn is %s.
Write 80 to 150 words in the voice of a terse radio operator summing up the turn for the players' side. Use only the facts in the report lines you're given: don't add units, casualties, places, enemy identities, or anything players weren't told. Enemy units are "contacts" unless the lines say otherwise. No headings, no lists, no sign-off; plain prose only.`, mod.Name, mod.Scale.Turn)
	user = fmt.Sprintf("Turn %d report lines:\n- %s", rep.Turn, strings.Join(rep.Players, "\n- "))
	return system, user, true
}

// ApplyNarrative stores a drafted narrative on its report for the referee.
func (r *Room) ApplyNarrative(run *Run, reply string) error {
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

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// --- Worker hooks ---

// NextRun marks the oldest pending run in the room as running and returns
// a copy, or nil. model and provider are recorded on it.
func (r *Room) NextRun(model, provider string, now time.Time) *Run {
	if !r.Assistant.On {
		return nil
	}
	for _, run := range r.Assistant.Runs {
		if run.Status == RunPending {
			run.Status, run.Started, run.Model, run.Provider, run.Error = RunRunning, now.UTC(), model, provider, ""
			run.Prompt = map[string]string{RunOrderReview: promptOrderReview, RunNarrative: promptNarrative}[run.Kind]
			c := *run
			return &c
		}
	}
	return nil
}

// HasPendingRun reports whether the worker has anything to do here.
func (r *Room) HasPendingRun() bool {
	if !r.Assistant.On {
		return false
	}
	return slices.ContainsFunc(r.Assistant.Runs, func(x *Run) bool { return x.Status == RunPending })
}

// FinishRun records a run's result. apply turns the reply into changes.
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
	case RunOrderReview:
		n := 0
		for _, is := range r.Assistant.Issues {
			if is.Turn == run.Turn && is.Source == "ai" && is.Status == IssueOpen {
				n++
			}
		}
		r.notify(ref.ID, fmt.Sprintf("Turn %d orders reviewed", run.Turn), fmt.Sprintf("The assistant raised %s.", plural(n, "issue", "issues")), "turn")
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
	return map[string]string{RunOrderReview: "Order review", RunNarrative: "Narrative"}[kind]
}

// --- Referee actions ---

// SetAssistant changes the room's assistant settings.
func (r *Room) SetAssistant(actor *Member, on, review, narrative bool) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	was := r.Assistant.On
	r.Assistant.On, r.Assistant.Review, r.Assistant.Narrative = on, review, narrative
	if on != was {
		r.logKind(VisReferee, "", "AI assistant switched %s.", map[bool]string{true: "on", false: "off"}[on])
	}
	return nil
}

// RunNow queues a review of the turn being resolved, or a narrative for the
// last report, on the referee's request.
func (r *Room) RunNow(actor *Member, kind string, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if !r.Assistant.On {
		return bad("switch the AI assistant on for this room first (Room screen)")
	}
	switch kind {
	case RunOrderReview:
		if r.Phase != PhaseResolving {
			return bad("orders can be reviewed once the turn is locked")
		}
		r.queueRun(RunOrderReview, r.Turn, now)
	case RunNarrative:
		if len(r.Reports) == 0 {
			return bad("there's no report to write a narrative for yet")
		}
		r.queueRun(RunNarrative, r.Reports[len(r.Reports)-1].Turn, now)
	default:
		return bad("unknown assistant task")
	}
	return nil
}

// DecideIssue is the referee noting, dismissing, or forwarding a card.
func (r *Room) DecideIssue(actor *Member, id, action, note string, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	i := slices.IndexFunc(r.Assistant.Issues, func(x *Issue) bool { return x.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	is := r.Assistant.Issues[i]
	note = clip(note, 300)
	switch action {
	case "noted":
		is.Status = IssueNoted
	case "dismiss":
		is.Status = IssueDismissed
	case "send":
		u := r.Unit(is.Unit)
		holder := (*Member)(nil)
		if u != nil {
			holder = r.memberOrNil(u.AssignedTo)
		}
		if holder == nil || holder.Status != StatusApproved {
			return bad("there's no player holding that unit to send it to")
		}
		text := note
		if text == "" {
			text = fmt.Sprintf("About %s's order for turn %d: %s", is.UnitName, is.Turn, is.Text)
			if is.Suggestion != "" {
				text += " " + is.Suggestion
			}
		}
		if err := r.Post(actor, "ref-"+holder.ID, text, now); err != nil {
			return err
		}
		is.Status = IssueSent
	case "reopen":
		is.Status = IssueOpen
	default:
		return bad("note, dismiss, or send")
	}
	is.Note, is.DecidedAt = note, now.UTC()
	return nil
}

// DecideNarrative is the referee approving, editing, or discarding a draft.
func (r *Room) DecideNarrative(actor *Member, op, turn int, action, text string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	rep := r.report(op, turn)
	if rep == nil || rep.Narrative == "" && action != "save" {
		return ErrNotFound
	}
	switch action {
	case "approve", "save":
		if t := strings.TrimSpace(text); t != "" {
			if t != rep.Narrative {
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
	Review    bool
	Narrative bool
	Runs      []Run       // latest first
	Issues    []IssueView // this turn's and the last, open first
	Open      int      // open cards
	Blocking  int      // open blocking cards
}

// IssueView is a card as shown, with whether it can go to a player.
type IssueView struct {
	Issue
	CanSend bool
}

func assistantView(r *Room) AIView {
	a := r.Assistant
	v := AIView{On: a.On, Review: a.Review, Narrative: a.Narrative}
	for i := len(a.Runs) - 1; i >= 0 && len(v.Runs) < 5; i-- {
		v.Runs = append(v.Runs, *a.Runs[i])
	}
	op := max(r.Operation, 1)
	for _, is := range a.Issues {
		if is.Operation == op && is.Turn >= r.Turn-1 {
			iv := IssueView{Issue: *is}
			if u := r.Unit(is.Unit); u != nil {
				if m := r.memberOrNil(u.AssignedTo); m != nil && m.Status == StatusApproved {
					iv.CanSend = true
				}
			}
			v.Issues = append(v.Issues, iv)
			if is.Status == IssueOpen {
				v.Open++
				if is.Severity == SeverityBlocking {
					v.Blocking++
				}
			}
		}
	}
	rank := map[string]int{SeverityBlocking: 0, SeverityJudgment: 1, SeverityInfo: 2}
	slices.SortStableFunc(v.Issues, func(x, y IssueView) int {
		if (x.Status == IssueOpen) != (y.Status == IssueOpen) {
			if x.Status == IssueOpen {
				return -1
			}
			return 1
		}
		if x.Turn != y.Turn {
			return y.Turn - x.Turn
		}
		return rank[x.Severity] - rank[y.Severity]
	})
	return v
}
