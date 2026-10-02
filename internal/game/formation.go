package game

import (
	"fmt"
	"strings"
)

// Formations. Units that move together share one token: a squad walking a
// street, a platoon, a Civil War brigade. Each member is still its own
// character (its own player, brief, card, and orders), but on the map they
// are one counter, moved as one. When one breaks away (a marine runs for the
// stairs) the referee splits them off: they get a small punch-out chit of
// their own, and the formation's counter shows a hole where they were.
// Rejoining puts them back in.

// Formation is a group of units that share a token.
type Formation struct {
	ID   string `json:"id"`
	Name string `json:"name"`           // "Lantern Squad"
	Abbr string `json:"abbr,omitempty"` // printed on the counter: "SQD"
}

// FormationToken is the id the map uses for a formation's counter.
func FormationToken(id string) string { return "f-" + id }

func (r *Room) formation(id string) *Formation {
	for _, f := range r.Formations {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// members are a formation's units, in order.
func (r *Room) members(fid string) []*Unit {
	var out []*Unit
	for _, u := range r.Units {
		if u.Formation == fid {
			out = append(out, u)
		}
	}
	return out
}

// together reports whether a unit is in its formation's token.
func (u *Unit) together() bool { return u.Formation != "" && !u.Detached }

// formationAnchor is where a formation's token stands: with its first
// member still in it. ok is false when every member has split off.
func (r *Room) formationAnchor(fid string, except *Unit) (Pos, bool) {
	for _, u := range r.members(fid) {
		if u != except && u.together() {
			return u.Pos(), true
		}
	}
	return Pos{}, false
}

// moveWithFormation keeps the members still in a formation's token on the
// same spot when one of them is moved: the token moves as one.
func (r *Room) moveWithFormation(u *Unit) {
	if !u.together() {
		return
	}
	for _, m := range r.members(u.Formation) {
		if m != u && m.together() && m.Pos() != u.Pos() {
			from := m.Pos()
			m.X, m.Y = u.X, u.Y
			r.noteMove(m, from)
		}
	}
}

// SplitOff takes a unit out of its formation's token: it gets a chit of its
// own, set just beside the formation.
func (r *Room) SplitOff(actor *Member, unitID string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	u := r.Unit(unitID)
	if u == nil {
		return ErrNotFound
	}
	if u.Formation == "" || u.Detached {
		return bad("%s isn't with a formation to split off from", u.Name)
	}
	from := u.Pos()
	u.Detached = true
	p := r.clampPos(Pos{u.X + 0.9*r.Board.Cell, u.Y + 0.35*r.Board.Cell})
	u.X, u.Y = p.X, p.Y
	r.noteMove(u, from)
	name := u.Formation
	if f := r.formation(u.Formation); f != nil {
		name = f.Name
	}
	r.logf(VisAll, "%s split off from %s.", u.Name, name)
	return nil
}

// Rejoin puts a unit back in its formation's token, wherever that stands.
func (r *Room) Rejoin(actor *Member, unitID string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	u := r.Unit(unitID)
	if u == nil {
		return ErrNotFound
	}
	if u.Formation == "" || !u.Detached {
		return bad("%s is already with their formation", u.Name)
	}
	from := u.Pos()
	if at, ok := r.formationAnchor(u.Formation, u); ok {
		u.X, u.Y = at.X, at.Y
	}
	u.Detached = false
	r.noteMove(u, from)
	name := u.Formation
	if f := r.formation(u.Formation); f != nil {
		name = f.Name
	}
	r.logf(VisAll, "%s rejoined %s.", u.Name, name)
	return nil
}

// squadForAI tells the model which formation a unit moves with.
func (r *Room) squadForAI(u *Unit) string {
	f := r.formation(u.Formation)
	if f == nil {
		return ""
	}
	if u.Detached {
		return fmt.Sprintf("split off from %s; moving on their own", f.Name)
	}
	var with []string
	for _, m := range r.members(f.ID) {
		if m != u && m.together() {
			with = append(with, m.Name)
		}
	}
	if len(with) == 0 {
		return "the last of " + f.Name
	}
	return fmt.Sprintf("with %s (moving together with %s)", f.Name, strings.Join(with, ", "))
}
