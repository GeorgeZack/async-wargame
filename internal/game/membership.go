package game

import (
	"fmt"
	"time"
)

// Leaving. A player who leaves, or whom the referee removes, loses access at
// once: their seat stops working and they stop receiving notifications. Their
// units, with any orders and drafts on them, pass to whoever the referee
// names; by default to the player's commander, or else back to the referee.
// The new holder can confirm, change, or withdraw those orders before the
// lock. A commander's players then report to the referee.

// Remove takes member id out of the operation, handing their units to heir
// ("" for their commander, or the referee if they have none; "referee" for
// the referee). The referee can remove anyone but themselves; a member can
// remove only themselves.
func (r *Room) Remove(actor *Member, id, heir string) error {
	m := r.memberOrNil(id)
	if m == nil || m.Role == RoleReferee || m.Status != StatusApproved {
		return bad("only an approved player or commander can leave the operation")
	}
	self := actor.ID == m.ID
	if actor.Role != RoleReferee && !self {
		return ErrForbidden
	}
	if self {
		heir = ""
	}
	var to *Member
	switch heir {
	case "":
		to = r.memberOrNil(m.ReportsTo)
	case "referee":
	default:
		to = r.memberOrNil(heir)
		if to == nil || to.ID == m.ID || to.Role == RoleReferee || to.Status != StatusApproved {
			return bad("the units can go to an approved player, a commander, or the referee")
		}
	}
	if to != nil && to.Status != StatusApproved {
		to = nil
	}

	if m.Role == RoleCommander {
		for _, s := range r.Subordinates(m) {
			s.ReportsTo = ""
		}
	}
	var units []string
	for _, u := range r.Units {
		if u.AssignedTo != m.ID {
			continue
		}
		units = append(units, u.Name)
		u.AssignedTo = ""
		if to != nil {
			u.AssignedTo = to.ID
		}
	}
	now := time.Now().UTC()
	m.Status, m.DecidedAt, m.Push = StatusRemoved, &now, nil
	verb := "was removed from"
	if self {
		verb = "left"
	}
	r.logf(VisAll, "%s %s the operation.", m.Name, verb)
	if len(units) > 0 {
		holder := "the referee"
		if to != nil {
			holder = to.Name
		}
		r.logf(VisAll, "%s %s now with %s, orders and all.", joinList(units), map[bool]string{true: "is", false: "are"}[len(units) == 1], holder)
		if to != nil {
			r.notify(to.ID, "Units handed to you", fmt.Sprintf("%s %s now yours. Check their orders before the lock.", joinList(units), map[bool]string{true: "is", false: "are"}[len(units) == 1]), "orders")
		}
	}
	return nil
}
