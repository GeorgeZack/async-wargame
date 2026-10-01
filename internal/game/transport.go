package game

import (
	"fmt"
	"slices"
)

// Transports. A friendly unit whose class has a Capacity can carry units
// whose class Embarks, up to that many people (by strength). A unit boards
// with an Embark order: it moves to the transport and climbs in at the end of
// its move, from the transport's hex on land or from a hex next to a
// transport on water (a dropship can't come ashore). Passengers ride along
// wherever the transport goes, don't count toward stacking, and get off with
// a Disembark order into a hex next to the transport (or under it, on land)
// at the start of the turn.

// TransportOption is a transport a unit could board this turn.
type TransportOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Hex   string `json:"hex"`
	Room  int    `json:"room"`  // people it can still take
	Board string `json:"board"` // hex the unit climbs in from
	Cost  int    `json:"cost"`
}

func isTransport(mod *Module, u *Unit) bool {
	return u != nil && mod.Side(u.Side).Playable && mod.Class(u.Class).Capacity > 0
}

// passengers lists the units aboard transport t.
func (r *Room) passengers(t *Unit) []*Unit {
	var out []*Unit
	for _, u := range r.Units {
		if u.Aboard == t.ID {
			out = append(out, u)
		}
	}
	return out
}

// roomAboard is how many more people transport t can take, counting units
// already aboard and this turn's embark orders for it that me can see (nil:
// all of them), except unit except.
func (r *Room) roomAboard(mod *Module, t *Unit, except string, me *Member) int {
	used := 0
	for _, u := range r.Units {
		if u.ID == except {
			continue
		}
		if u.Aboard == t.ID {
			used += u.Strength
			continue
		}
		if me != nil && !r.seesOrder(me, u) {
			continue
		}
		if o := r.CurrentOrder(u.ID); o != nil && o.Transport == t.ID && (o.Status == OrderDraft || o.Status == OrderCommitted) {
			used += u.Strength
		}
	}
	return mod.Class(t.Class).Capacity - used
}

// boardingHexes are the land hexes a unit can climb into t from.
func (r *Room) boardingHexes(mod *Module, t *Unit) [][2]int {
	if !mod.TerrainAt(r.Map.TerrainCode(t.Col, t.Row)).Water() {
		return [][2]int{{t.Col, t.Row}}
	}
	var out [][2]int
	for _, n := range r.Map.Neighbors(t.Col, t.Row) {
		if !mod.TerrainAt(r.Map.TerrainCode(n[0], n[1])).Water() {
			out = append(out, n)
		}
	}
	return out
}

// TransportsFor lists the transports unit u can reach and board this turn.
func (r *Room) TransportsFor(mod *Module, u *Unit, reach map[[2]int]Reach, me *Member) []TransportOption {
	if u.Aboard != "" || !mod.Class(u.Class).Embarks {
		return nil
	}
	var out []TransportOption
	for _, t := range r.Units {
		if t == u || !isTransport(mod, t) {
			continue
		}
		room := r.roomAboard(mod, t, u.ID, me)
		best, bestCost := [2]int{-1, -1}, 1<<30
		for _, h := range r.boardingHexes(mod, t) {
			if st, ok := reach[h]; ok && st.Cost < bestCost {
				best, bestCost = h, st.Cost
			}
		}
		if best[0] < 0 {
			continue
		}
		out = append(out, TransportOption{ID: t.ID, Name: t.Name, Hex: HexID(t.Col, t.Row), Room: room,
			Board: HexID(best[0], best[1]), Cost: bestCost})
	}
	return out
}

// disembarkReach is where a passenger can get off: next to its transport,
// or under it on land. Each costs a step from the transport's hex.
func (r *Room) disembarkReach(mod *Module, u *Unit) map[[2]int]Reach {
	t := r.Unit(u.Aboard)
	if t == nil {
		return nil
	}
	here := [2]int{t.Col, t.Row}
	out := map[[2]int]Reach{here: {Cost: 0, Prev: here}}
	blocked := map[[2]int]bool{}
	for _, e := range r.Units {
		if mod.Side(e.Side).Enemy && r.VisibleToPlayers(mod, e) {
			blocked[[2]int{e.Col, e.Row}] = true
		}
	}
	for _, n := range r.Map.Neighbors(t.Col, t.Row) {
		if !mod.TerrainAt(r.Map.TerrainCode(n[0], n[1])).Water() && !blocked[n] {
			out[n] = Reach{Cost: 1, Prev: here}
		}
	}
	return out
}

// syncPassengers keeps passengers with their transport. A passenger whose
// transport is gone is set down where it was.
func (r *Room) syncPassengers() {
	for _, u := range r.Units {
		if u.Aboard == "" {
			continue
		}
		if t := r.Unit(u.Aboard); t != nil {
			u.Col, u.Row = t.Col, t.Row
		} else {
			u.Aboard = ""
		}
	}
}

// board puts a unit that ended its move at a boarding hex aboard.
func (r *Room) board(mod *Module, u, t *Unit) error {
	if !slices.Contains(r.boardingHexes(mod, t), [2]int{u.Col, u.Row}) {
		return fmt.Errorf("%s isn't next to %s", u.Name, t.Name)
	}
	if room := mod.Class(t.Class).Capacity - aboardStrength(r, t); room < u.Strength {
		return fmt.Errorf("%s has room for %d, and %s needs %d", t.Name, room, u.Name, u.Strength)
	}
	u.Aboard = t.ID
	u.Col, u.Row = t.Col, t.Row
	return nil
}

func aboardStrength(r *Room, t *Unit) int {
	n := 0
	for _, p := range r.passengers(t) {
		n += p.Strength
	}
	return n
}
