package game

import (
	"container/heap"
	"math/rand/v2"
	"slices"
)

// The enemy AI is a fixed doctrine, not a language model. At each lock it
// plans one turn of movement for every infected unit from what the infected
// can sense, spending real movement points over the terrain and routing
// around rivers and ridges to their crossings. Its plan is an ordinary
// proposal: the referee can apply, discard, or let it apply on its own.

// doctrine is how one class behaves, from the module's AI section.
type doctrine struct {
	sense     int  // hexes within which it notices prey
	hunter    bool // prefers isolated playable units
	ambusher  bool // waits until prey is close
	wanders   bool // shuffles about when it senses nothing
	stays     bool // never moves on its own
	screams   bool // draws nearby units that answer calls toward its prey
	noiseDraw bool // answers a call
	callRange int
}

func doctrineFor(cl *Class) doctrine {
	a := cl.AI
	if a == nil || cl.Movement == 0 {
		return doctrine{stays: true}
	}
	d := doctrine{sense: max(1, a.Sense), wanders: a.Wanders, noiseDraw: a.AnswersCall,
		hunter: a.Prefers == "isolated", callRange: max(1, a.CallRange)}
	switch a.Behavior {
	case "ambush":
		d.ambusher, d.wanders = true, false
	case "call":
		d.screams, d.stays, d.wanders = true, true, false
	case "hold":
		d.stays, d.wanders = true, false
	}
	return d
}

// PlannedMove is one unit's intended path for the turn, start hex first.
type PlannedMove struct {
	Unit string
	Path [][2]int
}

// EnemyMoves plans the infected's paths for the current turn, by doctrine,
// from where everyone stands at lock. The resolution walks these paths
// slice by slice alongside the players' orders.
func (r *Room) EnemyMoves(mod *Module, rng *rand.Rand) []PlannedMove {
	type prey struct {
		col, row int
		player   bool
		isolated bool
	}
	var targets []prey
	players := r.marinePositions(mod)
	for _, u := range r.Units {
		side := mod.Side(u.Side)
		if side.Enemy || mod.Class(u.Class).Domain != "land" {
			continue
		}
		p := prey{col: u.Col, row: u.Row, player: side.Playable}
		if p.player {
			p.isolated = true
			for _, m := range players {
				if (m[0] != u.Col || m[1] != u.Row) && HexDistance(m[0], m[1], u.Col, u.Row) <= 2 {
					p.isolated = false
				}
			}
		}
		targets = append(targets, p)
	}
	anyIsolated := slices.ContainsFunc(targets, func(p prey) bool { return p.player && p.isolated })
	occupied := map[[2]int]bool{} // hexes held by people; the dead stop beside them
	for _, t := range targets {
		occupied[[2]int{t.col, t.row}] = true
	}

	// Callers (screamers) draw units that answer calls toward their prey.
	calls := map[string][2]int{}
	for _, u := range r.Units {
		cl := mod.Class(u.Class)
		d := doctrineFor(cl)
		if !mod.Side(u.Side).Enemy || !d.screams {
			continue
		}
		best, bd := [2]int{}, 1<<30
		for _, t := range targets {
			if dist := HexDistance(u.Col, u.Row, t.col, t.row); dist <= max(d.sense, 1) && dist < bd {
				best, bd = [2]int{t.col, t.row}, dist
			}
		}
		if bd == 1<<30 {
			continue
		}
		for _, h := range r.Units {
			if mod.Side(h.Side).Enemy && doctrineFor(mod.Class(h.Class)).noiseDraw && HexDistance(u.Col, u.Row, h.Col, h.Row) <= d.callRange {
				calls[h.ID] = best
			}
		}
	}

	var out []PlannedMove
	for _, u := range r.Units {
		if !mod.Side(u.Side).Enemy {
			continue
		}
		cl := mod.Class(u.Class)
		d := doctrineFor(cl)
		if d.stays {
			continue
		}
		var goal *[2]int
		if c, ok := calls[u.ID]; ok {
			goal = &c
		} else {
			bd := 1 << 30
			for _, t := range targets {
				dist := HexDistance(u.Col, u.Row, t.col, t.row)
				if dist > d.sense || d.hunter && t.player && !t.isolated && anyIsolated {
					continue
				}
				if d.hunter && !t.player {
					dist += 3 // hunters prefer soldiers
				}
				if dist < bd {
					g := [2]int{t.col, t.row}
					goal, bd = &g, dist
				}
			}
		}
		start := [2]int{u.Col, u.Row}
		switch {
		case goal != nil:
			if path := r.advance(mod, u.Col, u.Row, *goal, cl.Movement, occupied); len(path) > 1 {
				out = append(out, PlannedMove{Unit: u.ID, Path: path})
			}
		case d.wanders && rng.IntN(2) == 0:
			for _, dir := range rng.Perm(6) {
				nc, nr, ok := r.Map.Neighbor(u.Col, u.Row, dir)
				if ok && r.Map.GroundPassable(mod, u.Col, u.Row, dir) && !occupied[[2]int{nc, nr}] {
					out = append(out, PlannedMove{Unit: u.ID, Path: [][2]int{start, {nc, nr}}})
					break
				}
			}
		}
	}
	return out
}

// moveCost is the movement points to cross from (col,row) over side dir, or -1 if impassable.
func (r *Room) moveCost(mod *Module, col, row, dir int) int {
	if !r.Map.GroundPassable(mod, col, row, dir) {
		return -1
	}
	nc, nr, _ := r.Map.Neighbor(col, row, dir)
	cost := mod.TerrainAt(r.Map.TerrainCode(nc, nr)).Move
	if e := r.Map.EdgeAt(HexSide{col, row, dir}); e != nil {
		if et := mod.EdgeType(e.Type); et != nil {
			cost += et.Cost
		}
		if c := mod.Crossing(e.Crossing); c != nil {
			cost += c.Cost
		}
	}
	return cost
}

// advance moves a unit along its cheapest path toward goal, spending up to
// budget movement points and stopping beside the goal rather than in it.
func (r *Room) advance(mod *Module, col, row int, goal [2]int, budget int, occupied map[[2]int]bool) [][2]int {
	start := [2]int{col, row}
	dist := map[[2]int]int{start: 0}
	prev := map[[2]int][2]int{}
	pq := &hexQueue{{start, 0}}
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(hexItem)
		if cur.cost > dist[cur.hex] {
			continue
		}
		for dir := range 6 {
			c := r.moveCost(mod, cur.hex[0], cur.hex[1], dir)
			if c < 0 {
				continue
			}
			nc, nr, _ := r.Map.Neighbor(cur.hex[0], cur.hex[1], dir)
			n := [2]int{nc, nr}
			if occupied[n] {
				continue
			}
			if d, ok := dist[n]; !ok || cur.cost+c < d {
				dist[n], prev[n] = cur.cost+c, cur.hex
				heap.Push(pq, hexItem{n, cur.cost + c})
			}
		}
	}
	// Destination: the cheapest reachable hex next to the goal.
	dest, best := start, 1<<30
	if HexDistance(col, row, goal[0], goal[1]) <= 1 {
		return [][2]int{start} // already in contact
	}
	for h, c := range dist {
		if HexDistance(h[0], h[1], goal[0], goal[1]) == 1 && (c < best || c == best && hexLess(h, dest)) {
			dest, best = h, c
		}
	}
	if best == 1<<30 {
		return [][2]int{start} // no way through
	}
	var path [][2]int
	for h := dest; h != start; h = prev[h] {
		path = append(path, h)
	}
	slices.Reverse(path)
	out := [][2]int{start}
	for _, h := range path {
		if dist[h] > budget {
			break
		}
		out = append(out, h)
	}
	return out
}

func hexLess(a, b [2]int) bool { return a[1] < b[1] || a[1] == b[1] && a[0] < b[0] }

type hexItem struct {
	hex  [2]int
	cost int
}

type hexQueue []hexItem

func (q hexQueue) Len() int { return len(q) }
func (q hexQueue) Less(i, j int) bool {
	if q[i].cost != q[j].cost {
		return q[i].cost < q[j].cost
	}
	return hexLess(q[i].hex, q[j].hex)
}
func (q hexQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *hexQueue) Push(x any)   { *q = append(*q, x.(hexItem)) }
func (q *hexQueue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}
