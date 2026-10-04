package game

import (
	"strings"
	"unicode"
)

// Automatic exchanges have one set of dice for both participants. Human
// referees can still explicitly create distinct contests for multiple actions.
func (r *Room) validateAutoContest(order string, a PartyInput, b *PartyInput) error {
	other := ""
	if b != nil {
		other = b.Unit
	}
	if a.Unit != "" && a.Unit == other {
		return bad("a unit cannot oppose itself")
	}
	if order != "" {
		o := r.lockedOrder(order)
		if o == nil {
			return bad("unknown contest order")
		}
		involved := false
		for _, u := range r.OrderMembers(o) {
			involved = involved || u.ID == a.Unit || u.ID == other
		}
		if !involved {
			return bad("a contest for an order must involve one of that order's units")
		}
	}
	if a.Unit != "" && other != "" {
		for _, ct := range r.Desk.Contests {
			if ct.AUnit == a.Unit && ct.BUnit == other || ct.AUnit == other && ct.BUnit == a.Unit {
				return bad("exchange already exists as contest %s; omit this duplicate and reuse its dice at resolution", ct.ID)
			}
		}
	}
	return nil
}

func (r *Room) contestInvolvesOrder(c *Contest, order string) bool {
	if c.Order == order {
		return true
	}
	if o := r.lockedOrder(order); o != nil {
		for _, u := range r.OrderMembers(o) {
			if u.ID == c.AUnit || u.ID == c.BUnit {
				return true
			}
		}
	}
	return false
}

// A narrow, deterministic guard against exposing canonical secret identities.
// It cannot detect paraphrases or all tactical information in free-form prose.
func (r *Room) validatePublicIdentities(mod *Module, text string) error {
	words := func(s string) string {
		return " " + strings.Join(strings.FieldsFunc(strings.ToLower(s), func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsDigit(c) }), " ") + " "
	}
	text = words(text)
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable || r.PubliclyKnown(mod, u) && u.Identified {
			continue
		}
		name := words(u.Name)
		if name != "  " && strings.Contains(text, name) {
			return bad("public text names unrevealed or unidentified unit %s; use an anonymous contact description without its true name", u.ID)
		}
	}
	return nil
}
