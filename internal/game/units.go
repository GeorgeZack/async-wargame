package game

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// QualityOf is how good a unit is, as a bonus to its rolls: the referee's
// value for it if set, else its template's.
func QualityOf(mod *Module, u *Unit) int {
	if u.Quality != nil {
		return *u.Quality
	}
	if cl := mod.Class(u.Class); cl != nil {
		return cl.Quality
	}
	return 0
}

// VeteranOf is the suggested bonus for experience: +1 per operation
// survived, up to +2.
func VeteranOf(u *Unit) int { return min(2, u.Operations) }

// baseBonus is a unit's usual bonus before the situation, and what it's
// made of: "quality +2, veteran +1".
func baseBonus(mod *Module, u *Unit) (int, string) {
	q, v := QualityOf(mod, u), VeteranOf(u)
	var why []string
	if q != 0 {
		why = append(why, fmt.Sprintf("quality %+d", q))
	}
	if v != 0 {
		why = append(why, fmt.Sprintf("veteran %+d", v))
	}
	return q + v, strings.Join(why, ", ")
}

// KitOf is what a unit carries: its own kit if set, else its template's.
func KitOf(mod *Module, u *Unit) string {
	if u.Kit != nil {
		return *u.Kit
	}
	if cl := mod.Class(u.Class); cl != nil {
		return cl.Kit
	}
	return ""
}

// UnitEdit is the referee changing a unit during play.
type UnitEdit struct {
	Name       *string
	At         *Pos
	Strength   *int
	Status     *string
	Notes      *string
	Kit        *string
	Quality    *int
	Known      *bool
	KnownTo    *[]string // members who know it's there (when Known is false)
	Identified *bool
	Brief      *string
	RadioDown  *bool
	Remove     bool
}

// EditUnit applies a referee's change to a unit. During adjudication,
// players keep seeing the board as it was at the lock until publishing.
func (r *Room) EditUnit(actor *Member, mod *Module, id string, e UnitEdit) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	u := r.Unit(id)
	if u == nil {
		return ErrNotFound
	}
	playable := mod.Side(u.Side).Playable
	if e.Remove {
		r.Units = slices.DeleteFunc(r.Units, func(x *Unit) bool { return x == u })
		r.logKind(VisReferee, "", "%s removed from the map.", u.Name)
		return nil
	}
	if e.Name != nil {
		name, err := CleanName(*e.Name)
		if err != nil {
			return err
		}
		u.Name = name
	}
	if e.At != nil {
		from := u.Pos()
		p := r.clampPos(*e.At)
		u.X, u.Y = p.X, p.Y
		r.noteMove(u, from)
		r.moveWithFormation(u)
	}
	if e.Strength != nil {
		if *e.Strength < 0 || *e.Strength > 100000 {
			return bad("strength is 0 to 100000")
		}
		u.Strength = *e.Strength
	}
	for _, f := range []struct {
		in  *string
		out *string
		max int
	}{{e.Status, &u.Status, 200}, {e.Notes, &u.Notes, 2000}} {
		if f.in != nil {
			t, err := cleanText(*f.in, f.max)
			if err != nil {
				return err
			}
			*f.out = t
		}
	}
	if e.Kit != nil {
		t, err := cleanText(*e.Kit, 500)
		if err != nil {
			return err
		}
		if cl := mod.Class(u.Class); cl != nil && t == cl.Kit {
			u.Kit = nil
		} else {
			u.Kit = &t
		}
	}
	if e.Quality != nil {
		if *e.Quality < -5 || *e.Quality > 10 {
			return bad("quality is -5 to +10")
		}
		if cl := mod.Class(u.Class); cl != nil && cl.Quality == *e.Quality {
			u.Quality = nil
		} else {
			u.Quality = e.Quality
		}
	}
	if e.Known != nil && !playable {
		u.Known = *e.Known
	}
	if e.KnownTo != nil && !playable {
		var ids []string
		for _, id := range *e.KnownTo {
			if m := r.memberOrNil(id); m != nil && m.Role != RoleReferee && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		u.KnownTo = ids
	}
	if !playable && !u.Known && len(u.KnownTo) == 0 {
		u.Identified = false // nobody can know what it is if nobody knows it's there
	}
	if e.Brief != nil {
		t, err := cleanText(*e.Brief, 3000)
		if err != nil {
			return err
		}
		u.Brief = t
	}
	if e.Identified != nil && !playable {
		u.Identified = *e.Identified
		if u.Identified {
			u.Known = true
		}
	}
	if e.RadioDown != nil && playable {
		u.RadioDown = *e.RadioDown
	}
	return nil
}

// AddUnit puts a new unit from a template on the map during play.
func (r *Room) AddUnit(actor *Member, mod *Module, class, hex, name string, strength int, known bool) (*Unit, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	err := r.applyEdit(mod, EditOp{Op: "unit-add", Class: class, Hex: hex, Name: name, Strength: strength, Known: &known})
	if err != nil {
		return nil, err
	}
	return r.Units[len(r.Units)-1], nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// UnitMove puts one unit at a point.
type UnitMove struct {
	ID string
	At Pos
}

// MoveUnits moves several units at once, as one change: all of them or,
// if any can't be moved, none.
func (r *Room) MoveUnits(actor *Member, mod *Module, moves []UnitMove) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if len(moves) == 0 || len(moves) > 200 {
		return bad("move 1 to 200 units at a time")
	}
	// Validate the whole batch before changing any positions.
	destinations := map[string]Pos{}
	for _, m := range moves {
		if r.Unit(m.ID) == nil {
			return ErrNotFound
		}
		if math.IsNaN(m.At.X) || math.IsNaN(m.At.Y) || math.IsInf(m.At.X, 0) || math.IsInf(m.At.Y, 0) {
			return bad("choose a point on the map")
		}
	}
	for _, m := range moves {
		u := r.Unit(m.ID)
		if u.together() {
			at := r.clampPos(m.At)
			if previous, ok := destinations[u.Formation]; ok && previous != at {
				return bad("members of a formation move together; split them off to move separately")
			}
			destinations[u.Formation] = at
		}
	}
	for _, m := range moves {
		at := m.At
		if err := r.EditUnit(actor, mod, m.ID, UnitEdit{At: &at}); err != nil {
			return err
		}
	}
	return nil
}
