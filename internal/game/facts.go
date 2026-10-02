package game

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Facts: the world, built together. Anyone can propose something about the
// setting or their own marine: "Okafor carries a breaching charge", "the
// hospital has a loading dock on the canal side". The referee accepts it
// (perhaps changed), or turns it down with a reason. Accepted facts are true
// from then on, and everyone can read them, unless the referee keeps one
// between them and the proposer ("Lin knows the night nurse").

// Fact statuses.
const (
	FactProposed  = "proposed"
	FactAccepted  = "accepted"
	FactRejected  = "rejected"
	FactWithdrawn = "withdrawn"
)

const maxFactLen = 400

// Fact is one proposed or established piece of the fiction.
type Fact struct {
	ID        string    `json:"id"`
	Turn      int       `json:"turn"`
	Unit      string    `json:"unit,omitempty"` // about this unit, if any
	Text      string    `json:"text"`
	Proposed  string    `json:"proposed,omitempty"` // the original wording, if the referee changed it
	By        string    `json:"by"`                 // member id
	Status    string    `json:"status"`
	Reply     string    `json:"reply,omitempty"` // the referee's reason or comment
	Private   bool      `json:"private,omitempty"`
	At        time.Time `json:"at"`
	DecidedAt time.Time `json:"decidedAt,omitzero"`
}

// ProposeFact adds a proposal. The referee's own facts are accepted at once.
func (r *Room) ProposeFact(me *Member, mod *Module, unitID, text string, now time.Time) (*Fact, error) {
	if me.Status != StatusApproved {
		return nil, ErrForbidden
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil, bad("say what's true")
	}
	if len([]rune(text)) > maxFactLen {
		return nil, bad("keep a fact under %d characters", maxFactLen)
	}
	if unitID != "" {
		u := r.Unit(unitID)
		if u == nil || me.Role != RoleReferee && !mod.Side(u.Side).Playable {
			return nil, bad("facts can be about your own side's units, or about the world")
		}
	}
	open := 0
	for _, f := range r.Facts {
		if f.By == me.ID && f.Status == FactProposed {
			open++
		}
	}
	if open >= 10 {
		return nil, bad("you have 10 facts waiting for the referee already")
	}
	f := &Fact{ID: newID(), Turn: r.Turn, Unit: unitID, Text: text, By: me.ID, Status: FactProposed, At: now.UTC()}
	r.Facts = append(r.Facts, f)
	if me.Role == RoleReferee {
		f.Status, f.DecidedAt = FactAccepted, now.UTC()
		r.logf(VisAll, "New fact: %s", text)
		return f, nil
	}
	if ref := r.Referee(); ref != nil {
		r.notify(ref.ID, fmt.Sprintf("%s proposes a fact", me.Name), text, "facts")
	}
	return f, nil
}

// DecideFact is the referee accepting (with changed wording, if text is
// set) or rejecting a proposal.
func (r *Room) DecideFact(actor *Member, id, action, text, reply string, private bool, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	f := r.fact(id)
	if f == nil {
		return ErrNotFound
	}
	reply = clip(strings.TrimSpace(reply), 300)
	switch action {
	case "accept":
		if t := strings.Join(strings.Fields(text), " "); t != "" && t != f.Text {
			if len([]rune(t)) > maxFactLen {
				return bad("keep a fact under %d characters", maxFactLen)
			}
			if f.Proposed == "" {
				f.Proposed = f.Text
			}
			f.Text = t
		}
		f.Status, f.Private = FactAccepted, private
		if !private {
			r.logf(VisAll, "New fact: %s", f.Text)
		}
	case "reject":
		if f.Status != FactProposed {
			return bad("only a proposal can be turned down")
		}
		f.Status = FactRejected
	case "retract":
		if f.Status != FactAccepted {
			return bad("only an accepted fact can be retracted")
		}
		f.Status = FactRejected
		r.logf(VisAll, "The referee retracted a fact: %s", f.Text)
	default:
		return bad("accept, reject, or retract")
	}
	f.Reply, f.DecidedAt = reply, now.UTC()
	if by := r.memberOrNil(f.By); by != nil && by.ID != actor.ID {
		verb := map[string]string{"accept": "accepted", "reject": "turned down", "retract": "retracted"}[action]
		body := f.Text
		if reply != "" {
			body += " — " + reply
		}
		r.notify(by.ID, fmt.Sprintf("The referee %s your fact", verb), body, "facts")
	}
	return nil
}

// WithdrawFact lets a proposer take back a proposal.
func (r *Room) WithdrawFact(me *Member, id string) error {
	f := r.fact(id)
	if f == nil {
		return ErrNotFound
	}
	if f.By != me.ID || f.Status != FactProposed {
		return ErrForbidden
	}
	f.Status = FactWithdrawn
	return nil
}

func (r *Room) fact(id string) *Fact {
	i := slices.IndexFunc(r.Facts, func(f *Fact) bool { return f.ID == id })
	if i < 0 {
		return nil
	}
	return r.Facts[i]
}

// seesFact reports whether me may read a fact.
func (r *Room) seesFact(me *Member, f *Fact) bool {
	switch {
	case me.Role == RoleReferee, f.By == me.ID:
		return f.Status != FactWithdrawn
	case f.Status == FactAccepted:
		return !f.Private
	}
	return false
}

// FactView is a fact as shown.
type FactView struct {
	*Fact
	ByName   string
	UnitName string
	Mine     bool
}

// FactsFor lists the facts me may read, newest first; about unitID only,
// if set.
func (r *Room) FactsFor(me *Member, unitID string) []FactView {
	var out []FactView
	for i := len(r.Facts) - 1; i >= 0; i-- {
		f := r.Facts[i]
		if unitID != "" && f.Unit != unitID || !r.seesFact(me, f) {
			continue
		}
		fv := FactView{Fact: f, Mine: f.By == me.ID}
		if m := r.memberOrNil(f.By); m != nil {
			fv.ByName = m.Name
		}
		if u := r.Unit(f.Unit); u != nil {
			fv.UnitName = u.Name
		}
		out = append(out, fv)
	}
	return out
}

// acceptedFacts are the facts everyone knows, for the assistant's prompts.
func (r *Room) acceptedFacts(unitID string) []string {
	var out []string
	for _, f := range r.Facts {
		if f.Status == FactAccepted && (unitID == "" || f.Unit == unitID) {
			out = append(out, f.Text)
		}
	}
	return out
}
