package game

import (
	"fmt"
	"slices"
	"strings"
)

// SkillOf is a unit's skill in percent: its own value if the referee set
// one, else its template's. Zero means it can't really do that.
func SkillOf(mod *Module, u *Unit, skill string) int {
	if v, ok := u.Skills[skill]; ok {
		return v
	}
	if cl := mod.Class(u.Class); cl != nil {
		return cl.Skills[skill]
	}
	return 0
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

// SkillLine is one skill as shown on a card.
type SkillLine struct {
	ID, Name, Note string
	Value          int
	Changed        bool // differs from the template
}

// SkillLines lists a unit's skills in module order, leaving out ones it
// doesn't have.
func SkillLines(mod *Module, u *Unit) []SkillLine {
	cl := mod.Class(u.Class)
	var out []SkillLine
	for _, s := range mod.Skills {
		v := SkillOf(mod, u, s.ID)
		if v == 0 {
			continue
		}
		out = append(out, SkillLine{ID: s.ID, Name: s.Name, Note: s.Note, Value: v, Changed: cl != nil && cl.Skills[s.ID] != v})
	}
	return out
}

// skillsText is a unit's skills in a line: "Shoot 60, Fight 50, ...".
func skillsText(mod *Module, u *Unit) string {
	var parts []string
	for _, s := range SkillLines(mod, u) {
		parts = append(parts, fmt.Sprintf("%s %d", s.Name, s.Value))
	}
	return strings.Join(parts, ", ")
}

// UnitEdit is the referee changing a unit during play.
type UnitEdit struct {
	Name       *string
	Hex        *string
	Strength   *int
	Status     *string
	Notes      *string
	Kit        *string
	Skills     map[string]int // only the ones to change
	Known      *bool
	Identified *bool
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
	if e.Hex != nil {
		col, row, err := r.hex(*e.Hex)
		if err != nil {
			return err
		}
		u.Col, u.Row = col, row
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
	for id, v := range e.Skills {
		if mod.Skill(id) == nil {
			return bad("unknown skill %q", id)
		}
		if v < 0 || v > 100 {
			return bad("skills are 0 to 100")
		}
		if u.Skills == nil {
			u.Skills = map[string]int{}
		}
		if cl := mod.Class(u.Class); cl != nil && cl.Skills[id] == v {
			delete(u.Skills, id)
		} else {
			u.Skills[id] = v
		}
	}
	if e.Known != nil && !playable {
		u.Known = *e.Known
		if !u.Known {
			u.Identified = false
		}
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
