package game

import (
	"fmt"
	"slices"
)

// The command net. A friendly unit is in contact if the module's comms rules
// connect it to a node (the beachhead, a dropship, a manned relay), directly
// or through other units' radios. The referee can force a unit in or out of
// contact ("lost on patrol"), random events can jam radios for a few turns,
// and the referee can switch the rules off for a room.
//
// Out of contact, a unit's orders stay with the player holding it: their
// commander can't see or change them, and other players can't see them. The
// referee isn't affected.

// Radio overrides, set by the referee on a unit.
const (
	RadioAuto = ""
	RadioUp   = "up"
	RadioDown = "down"
)

// CommsActive reports whether comms rules apply in this room.
func (r *Room) CommsActive(mod *Module) bool { return mod.Comms != nil && !r.CommsOff }

// RefreshComms works out who is on the net and reports changes. The store
// calls it after every update, before fog.
func (r *Room) RefreshComms(mod *Module) {
	var friendly []*Unit
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable {
			friendly = append(friendly, u)
		}
	}
	jammed := func(u *Unit) bool { return u.JammedUntil >= r.Turn && u.JammedUntil > 0 }
	contact := map[string]bool{}
	switch {
	case !r.CommsActive(mod):
		for _, u := range friendly {
			contact[u.ID] = u.Radio != RadioDown && !jammed(u)
		}
	default:
		c := mod.Comms
		var frontier []*Unit
		reach := func(u *Unit) {
			if !contact[u.ID] {
				contact[u.ID] = true
				frontier = append(frontier, u)
			}
		}
		usable := func(u *Unit) bool { return u.Radio != RadioDown && !jammed(u) }
		for _, u := range friendly {
			if u.Radio == RadioUp {
				reach(u)
			}
		}
		for _, n := range c.Nodes {
			var centers [][2]int
			if n.Feature != "" {
				for _, f := range r.Features {
					if f.Kind != n.Feature {
						continue
					}
					if n.Manned && !slices.ContainsFunc(friendly, func(u *Unit) bool { return HexDistance(u.Col, u.Row, f.Col, f.Row) <= 1 }) {
						continue
					}
					centers = append(centers, [2]int{f.Col, f.Row})
				}
			}
			if n.Class != "" {
				for _, u := range friendly {
					if u.Class == n.Class && usable(u) {
						centers = append(centers, [2]int{u.Col, u.Row})
					}
				}
			}
			for _, ctr := range centers {
				for _, u := range friendly {
					if usable(u) && HexDistance(u.Col, u.Row, ctr[0], ctr[1]) <= n.Range {
						reach(u)
					}
				}
			}
		}
		for len(frontier) > 0 { // radios pass the net along
			a := frontier[0]
			frontier = frontier[1:]
			for _, b := range friendly {
				if usable(b) && HexDistance(a.Col, a.Row, b.Col, b.Row) <= c.Relay {
					reach(b)
				}
			}
		}
	}
	for _, u := range friendly {
		in := contact[u.ID]
		switch {
		case in:
			u.LastContact = r.Turn
			if u.OutOfContact {
				u.OutOfContact = false
				if r.Phase != PhaseSetup {
					r.logf(VisAll, "%s is back on the command net.", u.Name)
				}
			}
		case !u.OutOfContact:
			u.OutOfContact = true
			if r.Phase != PhaseSetup {
				r.logf(VisAll, "Lost contact with %s at %s (%s).", u.Name, HexID(u.Col, u.Row), r.Map.SectorAt(u.Col, u.Row))
			}
		}
	}
}

// SetRadio is the referee forcing a unit in or out of contact, or back to auto.
func (r *Room) SetRadio(actor *Member, mod *Module, unitID, radio string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	u := r.Unit(unitID)
	if u == nil || !mod.Side(u.Side).Playable {
		return bad("only friendly units have radios")
	}
	if !slices.Contains([]string{RadioAuto, RadioUp, RadioDown}, radio) {
		return bad("radio is auto, up, or down")
	}
	u.Radio = radio
	r.logKind(VisReferee, "", "%s's radio set to %s.", u.Name, map[string]string{"": "auto", "up": "always in contact", "down": "out of contact"}[radio])
	return nil
}

// SetCommsRules switches the comms rules on or off for this room.
func (r *Room) SetCommsRules(actor *Member, on bool) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	r.CommsOff = !on
	if on {
		r.logf(VisAll, "The referee switched communications rules on: units off the command net keep their orders to themselves.")
	} else {
		r.logf(VisAll, "The referee switched communications rules off: every unit is on the net.")
	}
	return nil
}

// radioText describes a unit's contact for cards and lists.
func (r *Room) radioText(u *Unit) string {
	switch {
	case !u.OutOfContact:
		return "In contact"
	case u.JammedUntil >= r.Turn && u.JammedUntil > 0:
		return fmt.Sprintf("Radio jammed until turn %d", u.JammedUntil+1)
	case u.Radio == RadioDown:
		return "Out of contact"
	case u.LastContact > 0:
		return fmt.Sprintf("Out of contact since turn %d", u.LastContact)
	}
	return "Out of contact"
}
