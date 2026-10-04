package game

import (
	"slices"
	"strings"
	"time"
)

// Squad orders retain per-member records for readiness, permissions, and turn
// history. Their shared Group makes them one instruction on the desk. Writing
// an individual order removes that member from the shared instruction.
func (r *Room) CanOrderSquad(mod *Module, actor *Member, fid string) bool {
	members := r.livingMembers(fid)
	if r.formation(fid) == nil || len(members) < 2 {
		return false
	}
	for _, u := range members {
		if !r.CanOrder(mod, actor, u) || !r.seesOrder(actor, u) {
			return false
		}
	}
	return true
}

func (r *Room) SquadOrder(fid string) *Order {
	for _, u := range r.livingMembers(fid) {
		if o := r.CurrentOrder(u.ID); o != nil && o.Group == fid {
			return o
		}
	}
	return nil
}

func (r *Room) SaveSquadOrder(actor *Member, mod *Module, fid, text string, mark *Pos, commit bool, now time.Time) (*Order, error) {
	if err := r.ordersOpen(now); err != nil {
		return nil, err
	}
	if !r.CanOrderSquad(mod, actor, fid) {
		return nil, ErrForbidden
	}
	// Existing individual orders remain exceptions when the global order changes.
	var members []*Unit
	for _, u := range r.livingMembers(fid) {
		if o := r.CurrentOrder(u.ID); o == nil || o.Group == fid {
			members = append(members, u)
		}
	}
	if len(members) == 0 {
		return nil, bad("every member has an individual order; withdraw one to use a squad order")
	}
	var first *Order
	for _, u := range members {
		o, err := r.SaveOrder(actor, mod, u.ID, text, mark, commit, now)
		if err != nil {
			return nil, err
		}
		o.Group = fid
		if first == nil {
			first = o
		}
	}
	return first, nil
}

// OrderMembers is the effective membership of a shared instruction this turn.
func (r *Room) OrderMembers(o *Order) []*Unit {
	if o.Group == "" {
		if u := r.Unit(o.Unit); u != nil {
			return []*Unit{u}
		}
		return nil
	}
	var members []*Unit
	for _, u := range r.members(o.Group) {
		if current := r.CurrentOrder(u.ID); current != nil && current.Group == o.Group && current.Status == o.Status {
			members = append(members, u)
		}
	}
	return members
}

func (r *Room) orderName(o *Order) string {
	if o.Group != "" {
		if f := r.formation(o.Group); f != nil {
			return f.Name
		}
	}
	if u := r.Unit(o.Unit); u != nil {
		return u.Name
	}
	return "Unit"
}

func (r *Room) orderMemberNames(o *Order) string {
	var names []string
	for _, u := range r.OrderMembers(o) {
		names = append(names, u.Name)
	}
	return strings.Join(names, ", ")
}

// sharedOrderPeers validates the complete group before a withdrawal/review can
// change it, so a member cannot edit everyone else's instruction by its ID.
func (r *Room) sharedOrderPeers(o *Order) []*Order {
	if o.Group == "" {
		return []*Order{o}
	}
	var out []*Order
	for _, u := range r.OrderMembers(o) {
		out = append(out, r.CurrentOrder(u.ID))
	}
	return out
}

func (r *Room) restoreSquadOrder(actor *Member, mod *Module, u *Unit, now time.Time) error {
	if u.Formation == "" {
		return nil
	}
	shared := r.SquadOrder(u.Formation)
	if shared == nil || (shared.Status != OrderDraft && shared.Status != OrderCommitted) {
		return nil
	}
	// The individual member can rejoin only a shared order they can already read.
	if lead := r.Unit(shared.Unit); lead == nil || !r.seesOrder(actor, lead) {
		return nil
	}
	text, mark, commit, group := shared.Text, shared.Mark, shared.Status == OrderCommitted, shared.Group
	o, err := r.SaveOrder(actor, mod, u.ID, text, mark, commit, now)
	if err == nil {
		o.Group = group
	}
	return err
}

func containsHolder(holders []string, id string) bool { return slices.Contains(holders, id) }
