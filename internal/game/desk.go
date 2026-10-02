package game

import (
	"slices"
	"strings"
	"time"

	"asyncwargame/internal/rules"
)

// Adjudication. Once a turn locks, the referee works through it at the
// desk: they read each order, call for a roll wherever the outcome is in
// doubt (each side a d10 plus one bonus for everything that matters), and
// write what happened. The server rolls the dice (or the referee enters
// dice rolled at the table). The referee moves
// counters and changes units as they go, but players keep seeing the board
// as it was at the lock until the turn is published, when everyone gets
// the report.

// Desk is the referee's work on one turn.
type Desk struct {
	Turn     int        `json:"turn"`
	Contests []*Contest `json:"contests,omitempty"`
	Outcomes []*Outcome `json:"outcomes,omitempty"`
	// Questions about orders, by order id: drafted, and sent to the player.
	Questions map[string]*Question `json:"questions,omitempty"`
}

// Contest is a roll: one side against another, or one side alone.
type Contest struct {
	ID     string  `json:"id"`
	Order  string  `json:"order,omitempty"` // the order it belongs to, if any
	Title  string  `json:"title"`           // "Okafor's grenade", "sneaking past the lobby"
	A      Party   `json:"a"`
	AUnit  string  `json:"aUnit,omitempty"`
	B      *Party  `json:"b,omitempty"`
	BUnit  string  `json:"bUnit,omitempty"`
	Result *Result `json:"result,omitempty"`
	Manual bool    `json:"manual,omitempty"` // dice entered by the referee
	// Secret contests aren't shown to players: a hidden enemy's stealth, say.
	Secret   bool      `json:"secret,omitempty"`
	Source   string    `json:"source,omitempty"` // "ai" when the assistant suggested it
	Model    string    `json:"model,omitempty"`
	RolledAt time.Time `json:"rolledAt,omitzero"`
}

// Outcome visibility.
const (
	SeenByAll     = "all"     // the whole squad hears about it
	SeenByHolder  = "holder"  // only the player (and their commander, if in contact)
	SeenByReferee = "referee" // a note for the record
)

// Outcome is what happened: for one order, or on its own (the infected
// moving, the building catching fire).
type Outcome struct {
	ID     string `json:"id"`
	Order  string `json:"order,omitempty"`
	Title  string `json:"title,omitempty"` // for outcomes without an order
	Text   string `json:"text"`
	Seen   string `json:"seen"`
	Source string `json:"source,omitempty"` // "ai" if drafted by the assistant
	Model  string `json:"model,omitempty"`
	Edited bool   `json:"edited,omitempty"` // the referee changed the draft
	Draft  string `json:"draft,omitempty"`  // an assistant draft offered beside the referee's own text
}

func (r *Room) desk() (*Desk, error) {
	if r.Phase != PhaseResolving || r.Desk == nil || r.Desk.Turn != r.Turn {
		return nil, bad("there's no turn being adjudicated")
	}
	return r.Desk, nil
}

func (d *Desk) contest(id string) *Contest {
	for _, c := range d.Contests {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (r *Room) lockedOrder(id string) *Order {
	for _, o := range r.Orders {
		if o.ID == id && o.Turn == r.Turn && o.Status == OrderLocked {
			return o
		}
	}
	return nil
}

// PartyInput is one side of a roll as the referee fills it in: a unit, or
// a name. Bonus, if set, is the whole bonus; if not, it's the unit's
// quality and veterancy plus Situation.
type PartyInput struct {
	Unit      string
	Name      string
	Bonus     *int
	Situation int    // added to the unit's usual bonus when Bonus isn't set
	Why       string // what went into the bonus
}

func (r *Room) party(mod *Module, in PartyInput) (Party, string, error) {
	p := Party{Name: strings.TrimSpace(in.Name), Why: clip(strings.TrimSpace(in.Why), 160)}
	unit := ""
	u := r.Unit(in.Unit)
	if u != nil {
		unit = u.ID
		if p.Name == "" {
			p.Name = u.Name
		}
	}
	switch {
	case in.Bonus != nil:
		p.Bonus = *in.Bonus
	case u != nil:
		base, why := baseBonus(mod, u)
		p.Bonus = base + in.Situation
		switch {
		case why != "" && p.Why != "":
			p.Why = why + ", " + p.Why
		case why != "":
			p.Why = why
		}
	default:
		p.Bonus = in.Situation
	}
	if p.Name == "" {
		return p, "", bad("say who's rolling")
	}
	if p.Bonus < -20 || p.Bonus > 20 {
		return p, "", bad("a bonus is -20 to +20")
	}
	p.Name = clip(p.Name, 60)
	return p, unit, nil
}

// AddContest sets up a roll. b may be nil for a check.
func (r *Room) AddContest(actor *Member, mod *Module, orderID, title string, a PartyInput, b *PartyInput, secret bool) (*Contest, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return nil, err
	}
	if orderID != "" && r.lockedOrder(orderID) == nil {
		return nil, bad("that order isn't part of this turn")
	}
	if len(d.Contests) >= 100 {
		return nil, bad("that's a lot of rolls for one turn; delete some first")
	}
	c := &Contest{ID: newID(), Order: orderID, Title: clip(title, 80), Secret: secret}
	if c.A, c.AUnit, err = r.party(mod, a); err != nil {
		return nil, err
	}
	if b != nil {
		p, unit, err := r.party(mod, *b)
		if err != nil {
			return nil, err
		}
		c.B, c.BUnit = &p, unit
	}
	if c.Title == "" {
		c.Title = c.A.Name
		if c.B != nil {
			c.Title += " against " + c.B.Name
		}
	}
	d.Contests = append(d.Contests, c)
	return c, nil
}

// RollContest rolls a contest's dice on the server, or takes dice the
// referee rolled at the table (dieA > 0). Rolling again replaces the
// result: the referee is in charge.
func (r *Room) RollContest(actor *Member, id string, dieA, dieB int, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return err
	}
	c := d.contest(id)
	if c == nil {
		return ErrNotFound
	}
	var res Result
	if dieA > 0 {
		if res, err = rules.Resolve(c.A, dieA, c.B, dieB); err != nil {
			return err
		}
		c.Manual = true
	} else {
		rng, _ := newRNG()
		res, c.Manual = rules.Contest(rng, c.A, c.B), false
	}
	c.Result, c.RolledAt = &res, now.UTC()
	return nil
}

// DeleteContest removes a roll.
func (r *Room) DeleteContest(actor *Member, id string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return err
	}
	n := len(d.Contests)
	d.Contests = slices.DeleteFunc(d.Contests, func(c *Contest) bool { return c.ID == id })
	if len(d.Contests) == n {
		return ErrNotFound
	}
	return nil
}

// SetOutcome writes what happened for an order (orderID), or on its own
// (id of an existing standalone outcome, or both empty for a new one).
func (r *Room) SetOutcome(actor *Member, orderID, id, title, text, seen string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	d, err := r.desk()
	if err != nil {
		return err
	}
	if !slices.Contains([]string{SeenByAll, SeenByHolder, SeenByReferee}, seen) {
		return bad("say who learns about it")
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if len([]rune(text)) > 3000 {
		return bad("keep an outcome under 3000 characters")
	}
	var o *Outcome
	switch {
	case orderID != "":
		if r.lockedOrder(orderID) == nil {
			return bad("that order isn't part of this turn")
		}
		o = d.outcomeFor(orderID)
		if o == nil {
			o = &Outcome{ID: newID(), Order: orderID}
			d.Outcomes = append(d.Outcomes, o)
		}
	case id != "":
		if o = d.outcome(id); o == nil {
			return ErrNotFound
		}
	default:
		o = &Outcome{ID: newID()}
		d.Outcomes = append(d.Outcomes, o)
	}
	if text == "" && orderID == "" {
		d.Outcomes = slices.DeleteFunc(d.Outcomes, func(x *Outcome) bool { return x == o })
		return nil
	}
	if o.Source == "ai" && text != o.Text {
		o.Edited = true
	}
	o.Text, o.Seen = text, seen
	if orderID == "" {
		o.Title = clip(title, 80)
	}
	return nil
}

func (d *Desk) outcomeFor(orderID string) *Outcome {
	for _, o := range d.Outcomes {
		if o.Order == orderID {
			return o
		}
	}
	return nil
}

func (d *Desk) outcome(id string) *Outcome {
	for _, o := range d.Outcomes {
		if o.ID == id {
			return o
		}
	}
	return nil
}

// Unwritten counts locked orders with no outcome yet.
func (r *Room) Unwritten() int {
	if r.Desk == nil || r.Phase != PhaseResolving {
		return 0
	}
	n := 0
	for _, o := range r.LockedOrders() {
		if oc := r.Desk.outcomeFor(o.ID); oc == nil || oc.Text == "" {
			n++
		}
	}
	return n
}

// --- Publishing ---

// TurnReport is what a published turn says. Each entry has its own
// visibility; players see the ones meant for them.
type TurnReport struct {
	Operation int           `json:"operation"`
	Turn      int           `json:"turn"`
	At        time.Time     `json:"at"`
	Entries   []ReportEntry `json:"entries"`
	Corrected string        `json:"corrected,omitempty"` // why the referee reopened and republished it
	// An opening narrative: drafted by the assistant (or the referee), and
	// shown to players only once approved.
	Narrative       string `json:"narrative,omitempty"`
	NarrativeStatus string `json:"narrativeStatus,omitempty"`
	NarrativeModel  string `json:"narrativeModel,omitempty"`
	NarrativeEdited bool   `json:"narrativeEdited,omitempty"`
}

// ReportEntry is one order's outcome, or a standalone one.
type ReportEntry struct {
	Title  string   `json:"title"`
	Unit   string   `json:"unit,omitempty"`
	Holder string   `json:"holder,omitempty"` // member id holding the unit at the time
	Order  string   `json:"order,omitempty"`  // what they ordered
	Text   string   `json:"text"`
	Rolls  []string `json:"rolls,omitempty"`  // contests players may see
	Secret []string `json:"secret,omitempty"` // contests only the referee sees
	Seen   string   `json:"seen"`
}

// PublishTurn turns the desk into a report and opens the next turn.
func (r *Room) PublishTurn(actor *Member, mod *Module, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhaseResolving {
		return bad("the turn hasn't been locked yet")
	}
	r.publish(mod, now)
	return nil
}

func (r *Room) publish(mod *Module, now time.Time) {
	r.snapshot(now)
	d := r.Desk
	if d == nil || d.Turn != r.Turn {
		d = &Desk{Turn: r.Turn}
	}
	rep := &TurnReport{Operation: max(r.Operation, 1), Turn: r.Turn, At: now.UTC(), Corrected: r.pendingCorrection()}
	rolls := func(orderID string, e *ReportEntry) {
		for _, c := range d.Contests {
			if c.Order != orderID || c.Result == nil {
				continue
			}
			line := c.Title + ": " + c.Result.Text()
			if c.Secret {
				e.Secret = append(e.Secret, line)
			} else {
				e.Rolls = append(e.Rolls, line)
			}
		}
	}
	for _, o := range r.LockedOrders() {
		u := r.Unit(o.Unit)
		e := ReportEntry{Order: o.Text, Seen: SeenByAll, Unit: o.Unit}
		if u != nil {
			e.Title, e.Holder = u.Name, u.AssignedTo
			if u.OutOfContact {
				e.Seen = SeenByHolder
			}
		}
		if oc := d.outcomeFor(o.ID); oc != nil {
			e.Text, e.Seen = oc.Text, oc.Seen
		}
		rolls(o.ID, &e)
		rep.Entries = append(rep.Entries, e)
	}
	for _, oc := range d.Outcomes {
		if oc.Order != "" {
			continue
		}
		e := ReportEntry{Title: oc.Title, Text: oc.Text, Seen: oc.Seen}
		rep.Entries = append(rep.Entries, e)
	}
	// Rolls not tied to any order or outcome still go on the record.
	loose := ReportEntry{Title: "Other rolls", Seen: SeenByReferee}
	for _, c := range d.Contests {
		if c.Order == "" && c.Result != nil {
			loose.Secret = append(loose.Secret, c.Title+": "+c.Result.Text())
		}
	}
	if len(loose.Secret) > 0 {
		rep.Entries = append(rep.Entries, loose)
	}
	r.Reports = append(r.Reports, rep)
	r.LockedUnits = nil
	r.assistAtPublish(now)
	r.logKind(VisAll, "sitrep", "Turn %d is published. The report is under Situation.", r.Turn)
	r.openTurn(r.Turn+1, now)
}

// sees reports whether member me may read a report entry.
func (r *Room) seesEntry(me *Member, e ReportEntry) bool {
	switch {
	case me.Role == RoleReferee:
		return true
	case e.Seen == SeenByAll:
		return true
	case e.Seen == SeenByHolder:
		holder := r.memberOrNil(e.Holder)
		return e.Holder == me.ID || holder != nil && holder.ReportsTo == me.ID && me.Role == RoleCommander
	}
	return false
}

// ResultSummary is a short label for a rolled contest, for the desk.
func ResultSummary(c *Contest) string {
	if c.Result == nil {
		return "not rolled"
	}
	return c.Result.Short()
}
