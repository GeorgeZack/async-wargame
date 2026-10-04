package game

import "fmt"

// Radio contact. A marine is in contact unless the referee has marked their
// radio down (lost, broken, out of range in a basement) or an event has
// jammed it. Out of contact, a marine's orders stay with the player holding
// them, and the rest of the squad sees them where they last called in.

// RefreshComms works out who's in contact and reports changes. The store
// calls it after every update.
func (r *Room) RefreshComms(mod *Module) {
	visibility := VisAll
	if r.Phase == PhaseResolving {
		visibility = VisReferee // intermediate unit changes are unpublished
	}
	for _, u := range r.Units {
		if s := mod.Side(u.Side); s == nil || !s.Playable {
			continue
		}
		in := !u.RadioDown && !(u.JammedUntil > 0 && u.JammedUntil >= r.Turn)
		switch {
		case in:
			u.LastContact = r.Turn
			u.Reported = &Pos{u.X, u.Y}
			if u.OutOfContact {
				u.OutOfContact = false
				if r.Phase != PhaseSetup {
					r.logf(visibility, "%s is back in contact.", u.Name)
				}
			}
		case !u.OutOfContact:
			u.OutOfContact = true
			if r.Phase != PhaseSetup {
				r.logf(visibility, "Lost contact with %s.", u.Name)
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
		return fmt.Sprintf("Communications disrupted until turn %d", u.JammedUntil+1)
	case u.LastContact > 0:
		return fmt.Sprintf("Out of contact since turn %d", u.LastContact)
	}
	return "Out of contact"
}

// reportedAt is where players other than its holder think u is: where it
// last called in, if it's out of contact.
func (r *Room) reportedAt(u *Unit, me *Member) (Pos, bool) {
	if !u.OutOfContact || u.Reported == nil || me.Role == RoleReferee || u.AssignedTo == me.ID {
		return u.Pos(), false
	}
	return *u.Reported, *u.Reported != u.Pos()
}

// VisibleToPlayers reports whether any player knows u is there.
func (r *Room) VisibleToPlayers(mod *Module, u *Unit) bool {
	return mod.Side(u.Side).Playable || u.Known || len(u.KnownTo) > 0
}

// KnownBy reports whether member me knows u is there. The referee decides:
// friendly units are always known; others once revealed, to everyone or to
// particular players. A squad leader learns what their players learn.
func (r *Room) KnownBy(mod *Module, u *Unit, me *Member) bool {
	switch {
	case me.Role == RoleReferee, mod.Side(u.Side).Playable, u.Known:
		return true
	}
	for _, id := range u.KnownTo {
		if id == me.ID {
			return true
		}
		if m := r.memberOrNil(id); m != nil && me.Role == RoleCommander && m.ReportsTo == me.ID {
			return true
		}
	}
	return false
}

// PubliclyKnown distinguishes shared contacts from a discovery known only to a
// specific player. An automatic briefing must use the former audience.
func (r *Room) PubliclyKnown(mod *Module, u *Unit) bool { return mod.Side(u.Side).Playable || u.Known }
