package game

import (
	"fmt"
	"time"
)

// Standing orders. An order can be marked standing, and can name a task to
// take up on arrival ("move to the bridge, then dig in"). When a turn opens,
// each such order from the last turn comes back as a draft for the new turn:
// a move that hasn't arrived continues toward its destination from where the
// unit now stands; one that has becomes its follow-on task. Drafts lapse at
// lock, so a standing order still has to be confirmed every round, by the
// player or, for units they hold, the referee.

// carryOver brings last turn's standing orders forward as drafts.
func (r *Room) carryOver(mod *Module, now time.Time) {
	n := 0
	for _, u := range r.Units {
		if !mod.Side(u.Side).Playable || r.CurrentOrder(u.ID) != nil {
			continue
		}
		var prev *Order
		for _, o := range r.Orders {
			if o.Unit == u.ID && o.Turn == r.Turn-1 && o.Status == OrderLocked && (o.Standing || o.Then != "") && o.Transport == "" {
				prev = o
			}
		}
		if prev == nil {
			continue
		}
		o := &Order{ID: newID(), Turn: r.Turn, Unit: u.ID, Author: prev.Author, Task: prev.Task, Intent: prev.Intent,
			Risk: prev.Risk, Fallback: prev.Fallback, Standing: prev.Standing, Then: prev.Then, Carried: true,
			FromCol: u.Col, FromRow: u.Row, Status: OrderDraft, Version: 1, UpdatedAt: now.UTC()}
		dest := [2]int{prev.DestCol, prev.DestRow}
		if prev.Goal != nil {
			dest = *prev.Goal
		}
		switch {
		case prev.HasDest && [2]int{u.Col, u.Row} != dest && u.Aboard == "":
			reach := r.Reach(mod, u)
			best, bestD, bestC := [2]int{u.Col, u.Row}, HexDistance(u.Col, u.Row, dest[0], dest[1]), 0
			for h, st := range reach {
				d := HexDistance(h[0], h[1], dest[0], dest[1])
				if d < bestD || d == bestD && st.Cost < bestC && h != best {
					best, bestD, bestC = h, d, st.Cost
				}
			}
			if best == [2]int{u.Col, u.Row} {
				continue // no way forward this turn
			}
			// The destination stays the order's goal; this turn's leg ends at best.
			o.HasDest, o.DestCol, o.DestRow, o.Cost = true, best[0], best[1], reach[best].Cost
			o.Route = routeTo(reach, [2]int{u.Col, u.Row}, best)
			if best != dest {
				o.Goal = &dest
			}
		case prev.Then != "":
			o.Task, o.Then = prev.Then, "" // arrived: take up the follow-on task
		case prev.HasDest:
			continue // a standing move that has arrived has nothing left to do
		}
		o.History = []OrderRevision{{Version: 1, At: now.UTC(), By: "standing order", Status: OrderDraft,
			Summary: fmt.Sprintf("carried over from turn %d; confirm to keep it", r.Turn-1)}}
		r.Orders = append(r.Orders, o)
		n++
	}
	if n > 0 {
		r.logKind(VisPlayers, "", "%s carried over as drafts. Confirm them before the lock, or they lapse.", plural(n, "standing order", "standing orders"))
	}
}

// ConfirmOrder commits a carried-over standing order, checking it afresh.
func (r *Room) ConfirmOrder(actor *Member, mod *Module, id string, now time.Time) error {
	var o *Order
	for _, x := range r.Orders {
		if x.ID == id && x.Turn == r.Turn {
			o = x
		}
	}
	if o == nil || o.Status != OrderDraft {
		return bad("there's no draft to confirm")
	}
	in := OrderInput{Unit: o.Unit, Task: o.Task, Intent: o.Intent, Risk: o.Risk, Fallback: o.Fallback,
		Standing: o.Standing, Then: o.Then, Commit: true}
	if o.HasDest {
		in.Dest = HexID(o.DestCol, o.DestRow)
	}
	goal := o.Goal
	saved, err := r.SaveOrder(actor, mod, in, now)
	if err == nil {
		saved.Goal, saved.Carried = goal, true
	}
	return err
}

// ConfirmAllStanding confirms every carried-over draft the actor can see and order.
func (r *Room) ConfirmAllStanding(actor *Member, mod *Module, now time.Time) error {
	n := 0
	for _, o := range r.Orders {
		if o.Turn != r.Turn || o.Status != OrderDraft || !o.Carried {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil || !r.CanOrder(mod, actor, u) || !r.seesOrder(actor, u) {
			continue
		}
		if err := r.ConfirmOrder(actor, mod, o.ID, now); err != nil {
			return fmt.Errorf("%s: %w", u.Name, err)
		}
		n++
	}
	if n == 0 {
		return bad("no standing orders to confirm")
	}
	return nil
}
