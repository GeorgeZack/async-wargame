package game

import "fmt"

// Radio contact. A marine is in contact unless the referee has marked their
// radio down (lost, broken, out of range in a basement) or an event has
// jammed it. Out of contact, a marine's orders stay with the player holding
// them, and the rest of the squad sees them where they last called in.

// RefreshComms works out who's in contact and reports changes. The store
// calls it after every update.
func (r *Room) RefreshComms(mod *Module) {
	for _, u := range r.Units {
		if s := mod.Side(u.Side); s == nil || !s.Playable {
			continue
		}
		in := !u.RadioDown && !(u.JammedUntil > 0 && u.JammedUntil >= r.Turn)
		switch {
		case in:
			u.LastContact = r.Turn
			u.Reported = &[2]int{u.Col, u.Row}
			if u.OutOfContact {
				u.OutOfContact = false
				if r.Phase != PhaseSetup {
					r.logf(VisAll, "%s is back on the radio.", u.Name)
				}
			}
		case !u.OutOfContact:
			u.OutOfContact = true
			if r.Phase != PhaseSetup {
				r.logf(VisAll, "Lost contact with %s.", u.Name)
			}
		}
	}
}

// radioText describes a unit's contact for cards and lists.
func (r *Room) radioText(u *Unit) string {
	switch {
	case !u.OutOfContact:
		return "In contact"
	case u.JammedUntil >= r.Turn && u.JammedUntil > 0:
		return fmt.Sprintf("Radio jammed until turn %d", u.JammedUntil+1)
	case u.LastContact > 0:
		return fmt.Sprintf("Out of contact since turn %d", u.LastContact)
	}
	return "Out of contact"
}

// reportedAt is where players other than its holder think u is: where it
// last called in, if it's out of contact.
func (r *Room) reportedAt(u *Unit, me *Member) ([2]int, bool) {
	if !u.OutOfContact || u.Reported == nil || me.Role == RoleReferee || u.AssignedTo == me.ID {
		return [2]int{u.Col, u.Row}, false
	}
	return *u.Reported, *u.Reported != [2]int{u.Col, u.Row}
}

// VisibleToPlayers reports whether players know u is there. The referee
// decides: friendly units always are, others once revealed.
func (r *Room) VisibleToPlayers(mod *Module, u *Unit) bool {
	return mod.Side(u.Side).Playable || u.Known
}
