package game

import "time"

// The blind referee. During planning the referee doesn't see what players
// have ordered (routes, tasks, intent), so they can run the infected without
// bias, as the design doc asks. They still see positions and how many orders
// are in. Everything is revealed at lock. Admin mode shows everything, and
// switching it on during planning is announced to the players.

// VisPlayers entries are for players, and for the referee once the turn they
// belong to is no longer in planning (or in admin mode).
const VisPlayers Visibility = "players"

// Blind reports whether member me is a referee who can't see player orders now.
func (r *Room) Blind(me *Member) bool {
	return me.Role == RoleReferee && !me.AdminMode && r.Phase == PhasePlanning
}

// seesOrder reports whether me may see the order of unit u. A blind referee
// still sees orders for units they hold themselves.
func (r *Room) seesOrder(me *Member, u *Unit) bool {
	if u == nil {
		return true
	}
	if me.Role != RoleReferee {
		// Off the command net, a unit's orders stay with the player holding it.
		return !u.OutOfContact || u.AssignedTo == me.ID
	}
	if !r.Blind(me) {
		return true
	}
	return u.AssignedTo == "" || r.memberOrNil(u.AssignedTo) == nil
}

// seesLog reports whether me may read a log entry.
func (r *Room) seesLog(me *Member, e LogEntry) bool {
	switch e.Visibility {
	case VisAll:
		return true
	case VisReferee:
		return me.Role == RoleReferee
	case VisPlayers:
		if me.Role != RoleReferee {
			return true
		}
		return me.AdminMode || e.Turn != r.Turn || r.Phase != PhasePlanning
	}
	return false
}

// SetAdminMode switches the referee between blind and admin view.
func (r *Room) SetAdminMode(actor *Member, on bool, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if actor.AdminMode == on {
		return nil
	}
	actor.AdminMode = on
	switch {
	case on && r.Phase == PhasePlanning:
		r.logf(VisAll, "The referee switched to admin view and can see this turn's orders.")
	case on:
		r.logKind(VisReferee, "", "Admin view on.")
	default:
		r.logf(VisAll, "The referee is back in blind view: player orders stay hidden until lock.")
	}
	return nil
}
