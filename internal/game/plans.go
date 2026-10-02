package game

import (
	"math"
	"slices"
	"time"
)

// Plan is a private proposed destination. It never changes a unit's position.
type Plan struct {
	Turn   int    `json:"turn"`
	Member string `json:"member"`
	Unit   string `json:"unit"`
	To     Pos    `json:"to"`
}

func (r *Room) PlansFor(me *Member) []*Plan {
	var out []*Plan
	for _, p := range r.Plans {
		u := r.Unit(p.Unit)
		if p.Member == me.ID && p.Turn == r.Turn && u != nil && (me.Role == RoleReferee || u.AssignedTo == me.ID) {
			out = append(out, p)
		}
	}
	return out
}

func (r *Room) PlanUnits(actor *Member, mod *Module, moves []UnitMove, now time.Time) error {
	if actor.Status != StatusApproved || (actor.Role != RoleReferee && r.Phase != PhasePlanning) {
		return ErrForbidden
	}
	if len(moves) == 0 || len(moves) > 200 {
		return bad("plan 1 to 200 units at a time")
	}
	for _, m := range moves {
		u := r.Unit(m.ID)
		if u == nil {
			return ErrNotFound
		}
		if math.IsNaN(m.At.X) || math.IsNaN(m.At.Y) || math.IsInf(m.At.X, 0) || math.IsInf(m.At.Y, 0) {
			return bad("choose a point on the map")
		}
		if actor.Role != RoleReferee && (u.AssignedTo != actor.ID || !r.CanOrder(mod, actor, u) || !r.seesOrder(actor, u)) {
			return ErrForbidden
		}
	}
	// Referee staging moves attached formations as a whole. A later drag
	// replaces all their old destinations, so publish never sees conflicting pins.
	if actor.Role == RoleReferee {
		targets := map[string]Pos{}
		for _, m := range moves {
			u := r.Unit(m.ID)
			key := u.ID
			if u.together() {
				key = FormationToken(u.Formation)
			}
			at := r.clampPos(m.At)
			if old, ok := targets[key]; ok && old != at {
				return bad("attached squad members need the same destination; split a member off first")
			}
			targets[key] = at
		}
		expanded := []UnitMove{}
		for _, u := range r.Units {
			key := u.ID
			if u.together() {
				key = FormationToken(u.Formation)
			}
			if at, ok := targets[key]; ok {
				expanded = append(expanded, UnitMove{u.ID, at})
			}
		}
		moves = expanded
	}
	for _, m := range moves {
		at := r.clampPos(m.At)
		r.Plans = slices.DeleteFunc(r.Plans, func(p *Plan) bool { return p.Turn != r.Turn || p.Member == actor.ID && p.Unit == m.ID })
		r.Plans = append(r.Plans, &Plan{Turn: r.Turn, Member: actor.ID, Unit: m.ID, To: at})
	}
	updatedGroups := map[string]bool{}
	for _, m := range moves {
		at := r.clampPos(m.At)
		if actor.Role != RoleReferee {
			if o := r.CurrentOrder(m.ID); o != nil {
				target := m.ID
				if o.Group != "" && r.CanOrderSquad(mod, actor, o.Group) {
					together := true
					for _, member := range r.OrderMembers(o) {
						found := false
						for _, move := range moves {
							if move.ID == member.ID && r.clampPos(move.At) == at {
								found = true
								break
							}
						}
						if !found {
							together = false
							break
						}
					}
					if together {
						if updatedGroups[o.Group] {
							continue
						}
						updatedGroups[o.Group] = true
						target = FormationToken(o.Group)
					}
				}
				if _, err := r.SaveOrder(actor, mod, target, o.Text, &at, o.Status == OrderCommitted, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *Room) ApplyPlans(actor *Member, mod *Module) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	var moves []UnitMove
	for _, p := range r.PlansFor(actor) {
		moves = append(moves, UnitMove{ID: p.Unit, At: p.To})
	}
	if err := r.MoveUnits(actor, mod, moves); err != nil {
		return err
	}
	r.Plans = slices.DeleteFunc(r.Plans, func(p *Plan) bool { return p.Member == actor.ID && p.Turn == r.Turn })
	return nil
}

func (r *Room) ClearStagedMoves(actor *Member) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	r.Plans = slices.DeleteFunc(r.Plans, func(p *Plan) bool { return p.Member == actor.ID && p.Turn == r.Turn })
	return nil
}
