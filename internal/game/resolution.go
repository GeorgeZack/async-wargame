package game

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"asyncwargame/internal/rules"
)

// Turn resolution. At lock, every unit's path for the turn is known: players'
// units follow their committed orders, the infected follow the enemy AI, and
// escorted civilians go with their escort. The turn is split into the
// module's time slices. In each slice every moving unit advances as far as
// its movement points for that share of the turn allow; then any opposing
// units within one hex of each other stop and fight. That's simultaneous
// movement: nobody walks through anybody, and nobody gets a whole turn while
// the other side waits.
//
// The result is a proposal for the referee: where everyone ends up, and an
// encounter card for each fight with suggested dice pools. Nothing changes
// until the referee publishes the turn.

type Resolution struct {
	Turn       int          `json:"turn"`
	Seed       uint64       `json:"seed"`
	CreatedAt  time.Time    `json:"createdAt"`
	Slices     int          `json:"slices"`
	Moves      []Move       `json:"moves"`
	Encounters []*Encounter `json:"encounters"`
	Notes      []string     `json:"notes,omitempty"` // for the referee: missed orders, stacking
	Published  bool         `json:"published,omitempty"`
}

// Move is where a unit goes this turn and why it stopped.
type Move struct {
	Unit   string   `json:"unit"`
	Path   [][2]int `json:"path"` // hexes actually traversed, start first
	Slice  int      `json:"slice"`
	Reason string   `json:"reason"` // arrived, engaged, cautious, stacking
}

func (m Move) To() [2]int   { return m.Path[len(m.Path)-1] }
func (m Move) From() [2]int { return m.Path[0] }

// Encounter is one fight: everyone engaged together, at the slice it began.
type Encounter struct {
	ID      string   `json:"id"`
	Col     int      `json:"col"`
	Row     int      `json:"row"`
	Slice   int      `json:"slice"`
	Friend  []string `json:"friend"`  // playable units
	Neutral []string `json:"neutral"` // civilians caught up in it
	Enemy   []string `json:"enemy"`

	FriendSide rules.SideState  `json:"-"`
	EnemySide  rules.SideState  `json:"-"`
	Sit        [2]SideSituation `json:"situation"` // kept for the record and the desk
	Suggested  [2]rules.Pool    `json:"suggested"` // friendly, enemy
	Dice       [2]int           `json:"dice"`      // the pools actually rolled
	Faces      [2][]int         `json:"faces,omitempty"`
	Hits       [2]int           `json:"hits"`
	Margin     int              `json:"margin"`
	Winner     string           `json:"winner,omitempty"` // friend, enemy, or draw
	Outcome    *rules.Outcome   `json:"outcome,omitempty"`
	Settled    bool             `json:"settled"`
	Source     string           `json:"source,omitempty"` // rule engine or referee
	Reason     string           `json:"reason,omitempty"`
}

// SideSituation is what shaped one side's pool, in plain terms.
type SideSituation struct {
	Holding bool `json:"holding,omitempty"`
	Cover   int  `json:"cover,omitempty"`
	Across  bool `json:"across,omitempty"`
	Unseen  bool `json:"unseen,omitempty"`
}

func situationOf(s rules.SideState) SideSituation {
	return SideSituation{Holding: s.Holding, Cover: s.Cover, Across: s.Across, Unseen: s.Unseen}
}

// The encounter sides, as indexes into Suggested, Dice, Faces, and Hits.
const (
	FriendSide = 0
	EnemySide  = 1
)

type mover struct {
	unit      *Unit
	path      [][2]int
	costs     []int // movement to enter path[i]
	mp        int
	at        int
	spent     int
	engaged   int // slice, or 0
	careful   bool
	followers []*mover // escorted civilians and passengers moving with this one
}

// role of a unit's side: "player", "enemy", or "neutral".
func roleOf(mod *Module, u *Unit) string { return sideRole(mod.Side(u.Side)) }

// opposed reports whether two units fight when they meet.
func opposed(mod *Module, a, b *Unit) bool {
	ra, rb := roleOf(mod, a), roleOf(mod, b)
	return ra == "enemy" && rb != "enemy" || rb == "enemy" && ra != "enemy"
}

// resolve builds the turn's resolution from the locked orders and the enemy AI.
func (r *Room) resolve(mod *Module, now time.Time, rng *rand.Rand, seed uint64) *Resolution {
	res := &Resolution{Turn: r.Turn, Seed: seed, CreatedAt: now.UTC(), Slices: max(1, mod.Scale.Slices)}

	movers := map[string]*mover{}
	var order, escorts []*mover
	newMover := func(u *Unit, path [][2]int, careful bool) *mover {
		m := &mover{unit: u, path: path, mp: mod.Class(u.Class).Movement, careful: careful}
		m.costs = make([]int, len(path))
		for i := 1; i < len(path); i++ {
			for dir := range 6 {
				if nc, nr, ok := r.Map.Neighbor(path[i-1][0], path[i-1][1], dir); ok && nc == path[i][0] && nr == path[i][1] {
					m.costs[i] = max(1, r.stepCost(mod, mod.Class(u.Class), path[i-1][0], path[i-1][1], dir))
				}
			}
		}
		movers[u.ID] = m
		return m
	}
	addMover := func(u *Unit, path [][2]int, careful bool) *mover {
		m := newMover(u, path, careful)
		order = append(order, m)
		return m
	}

	// Players' units, by committed order.
	var missed []string
	for _, u := range r.Units {
		if !mod.Side(u.Side).Playable {
			continue
		}
		o := r.CurrentOrder(u.ID)
		if o == nil || o.Status != OrderLocked {
			if r.memberOrNil(u.AssignedTo) != nil {
				missed = append(missed, u.Name)
			}
			continue
		}
		if !o.HasDest || u.Col != o.FromCol || u.Row != o.FromRow || len(o.Route) == 0 {
			continue
		}
		path := append([][2]int{{u.Col, u.Row}}, o.Route...)
		if u.Aboard != "" { // getting off: one step, at the very start of the turn
			m := addMover(u, path, false)
			m.mp = 1000
			continue
		}
		careful := slices.ContainsFunc(mod.Orders.Risks, func(x Option) bool { return x.ID == o.Risk && x.StopsShort })
		if t := mod.Orders.Task(o.Task); t != nil && t.Careful {
			careful = true
		}
		m := addMover(u, path, careful)
		if slices.Contains(mod.Escort.Tasks, o.Task) {
			for _, c := range r.Units {
				if roleOf(mod, c) == "neutral" && c.Col == u.Col && c.Row == u.Row && movers[c.ID] == nil {
					cm := newMover(c, path, false) // follows its escort, never moves on its own
					m.followers = append(m.followers, cm)
					escorts = append(escorts, cm)
					break
				}
			}
		}
	}
	if len(missed) > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("No orders: %s hold and take no offensive action.", joinList(missed)))
	}
	// The infected, by doctrine.
	for _, pm := range r.EnemyMoves(mod, rng) {
		if u := r.Unit(pm.Unit); u != nil {
			addMover(u, pm.Path, false)
		}
	}

	// Passengers staying aboard ride with their transport.
	inside := map[string]bool{}
	for _, u := range r.Units {
		if u.Aboard == "" || movers[u.ID] != nil {
			continue
		}
		inside[u.ID] = true
		if tm := movers[u.Aboard]; tm != nil {
			pm := newMover(u, tm.path, false)
			tm.followers = append(tm.followers, pm)
			escorts = append(escorts, pm)
		}
	}

	pos := map[string][2]int{}
	start := map[string][2]int{}
	for _, u := range r.Units {
		pos[u.ID] = [2]int{u.Col, u.Row}
		start[u.ID] = pos[u.ID]
	}
	engagedAt := map[string]int{}
	pairs := map[[2]string]bool{}
	visibleAtLock := map[string]bool{}
	for _, u := range r.Units {
		visibleAtLock[u.ID] = r.VisibleToPlayers(mod, u)
	}

	detect := func(slice int) {
		for i, a := range r.Units {
			for _, b := range r.Units[i+1:] {
				if !opposed(mod, a, b) || inside[a.ID] || inside[b.ID] {
					continue
				}
				pa, pb := pos[a.ID], pos[b.ID]
				if HexDistance(pa[0], pa[1], pb[0], pb[1]) > 1 {
					continue
				}
				for _, id := range []string{a.ID, b.ID} {
					if engagedAt[id] == 0 {
						engagedAt[id] = slice
						if m := movers[id]; m != nil {
							m.engaged = slice
						}
					}
				}
				key := [2]string{a.ID, b.ID}
				pairs[key] = true
			}
		}
	}
	occupiedByFoe := func(u *Unit, h [2]int) bool {
		for _, o := range r.Units {
			if pos[o.ID] == h && opposed(mod, u, o) && !inside[o.ID] {
				return true
			}
		}
		return false
	}
	nearFoe := func(u *Unit, h [2]int) bool {
		for _, o := range r.Units {
			if p := pos[o.ID]; opposed(mod, u, o) && !inside[o.ID] && HexDistance(p[0], p[1], h[0], h[1]) <= 1 {
				return true
			}
		}
		return false
	}

	detect(1) // already in contact at lock: the fight starts at once
	for s := 1; s <= res.Slices; s++ {
		list := order
		if s%2 == 0 { // alternate who steps first within a slice
			list = slices.Clone(order)
			slices.Reverse(list)
		}
		for _, m := range list {
			if m.engaged > 0 || engagedAt[m.unit.ID] > 0 {
				continue
			}
			allowance := m.mp * s / res.Slices
			for m.at < len(m.path)-1 && m.spent+m.costs[m.at+1] <= allowance {
				next := m.path[m.at+1]
				if occupiedByFoe(m.unit, next) {
					break
				}
				if m.careful && nearFoe(m.unit, next) {
					m.careful = false // stops once, short of the contact
					m.path = m.path[:m.at+1]
					break
				}
				m.at++
				m.spent += m.costs[m.at]
				pos[m.unit.ID] = next
				for _, f := range m.followers {
					if f.engaged == 0 {
						f.at, pos[f.unit.ID] = m.at, next
					}
				}
			}
		}
		detect(s)
	}

	// Moves, with friendly stacking checked in the order orders were committed.
	load := r.Load(mod)
	for _, m := range slices.Concat(order, escorts) {
		if m.at == 0 {
			continue
		}
		path := slices.Clone(m.path[:m.at+1])
		reason := "arrived"
		switch {
		case engagedAt[m.unit.ID] > 0:
			reason = "engaged"
		case m.at < len(m.path)-1:
			reason = "stopped"
		}
		if roleOf(mod, m.unit) == "player" && !inside[m.unit.ID] { // passengers ride, they don't stack
			load[path[0]] -= stackPoints(mod, m.unit)
			end := len(path) - 1
			for end > 0 && !r.fits(mod, m.unit, path[end], load) {
				end--
			}
			if end < len(path)-1 {
				res.Notes = append(res.Notes, fmt.Sprintf("%s stops at %s: %s was full.", m.unit.Name,
					HexID(path[end][0], path[end][1]), HexID(path[len(path)-1][0], path[len(path)-1][1])))
				path, reason = path[:end+1], "stacking"
				pos[m.unit.ID] = path[end]
			}
			load[path[len(path)-1]] += stackPoints(mod, m.unit)
		}
		if len(path) > 1 {
			res.Moves = append(res.Moves, Move{Unit: m.unit.ID, Path: path, Slice: max(m.engaged, engagedAt[m.unit.ID]), Reason: reason})
		}
	}

	res.Encounters = r.buildEncounters(mod, pairs, engagedAt, pos, start, visibleAtLock)
	return res
}

// buildEncounters groups engaged units that touch into encounters and works
// out each side's situation and suggested pool.
func (r *Room) buildEncounters(mod *Module, pairs map[[2]string]bool, engagedAt map[string]int,
	pos, start map[string][2]int, visibleAtLock map[string]bool) []*Encounter {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	for p := range pairs {
		parent[find(p[0])] = find(p[1])
	}
	groups := map[string][]string{}
	var roots []string
	for _, u := range r.Units { // unit order keeps this deterministic
		if engagedAt[u.ID] == 0 {
			continue
		}
		root := find(u.ID)
		if groups[root] == nil {
			roots = append(roots, root)
		}
		groups[root] = append(groups[root], u.ID)
	}
	var out []*Encounter
	for _, root := range roots {
		e := &Encounter{ID: newID(), Slice: 1 << 30}
		for _, id := range groups[root] {
			u := r.Unit(id)
			e.Slice = min(e.Slice, engagedAt[id])
			switch roleOf(mod, u) {
			case "player":
				e.Friend = append(e.Friend, id)
			case "enemy":
				e.Enemy = append(e.Enemy, id)
			default:
				e.Neutral = append(e.Neutral, id)
			}
		}
		anchor := e.Neutral
		if len(e.Friend) > 0 {
			anchor = e.Friend
		}
		if len(anchor) == 0 {
			anchor = e.Enemy
		}
		at := pos[anchor[0]]
		e.Col, e.Row = at[0], at[1]
		e.FriendSide = r.sideState(mod, slices.Concat(e.Friend, e.Neutral), e.Enemy, pos, start, true)
		e.EnemySide = r.sideState(mod, e.Enemy, slices.Concat(e.Friend, e.Neutral), pos, start, false)
		if len(e.Enemy) > 0 && !slices.ContainsFunc(e.Enemy, func(id string) bool { return visibleAtLock[id] }) {
			e.EnemySide.Unseen = true // nobody saw them coming
		}
		e.Suggested = [2]rules.Pool{mod.Combat.Pool(e.FriendSide), mod.Combat.Pool(e.EnemySide)}
		e.Sit = [2]SideSituation{situationOf(e.FriendSide), situationOf(e.EnemySide)}
		e.Dice = [2]int{e.Suggested[0].Dice, e.Suggested[1].Dice}
		out = append(out, e)
	}
	return out
}

// sideState describes one side of a fight for the combat engine.
func (r *Room) sideState(mod *Module, ids, foes []string, pos, start map[string][2]int, friendly bool) rules.SideState {
	s := rules.SideState{Holding: true, Across: len(foes) > 0}
	for _, id := range ids {
		u := r.Unit(id)
		cl := mod.Class(u.Class)
		f := rules.Fighter{Name: u.Name, Class: cl, Strength: u.Strength}
		if friendly && roleOf(mod, u) == "player" {
			f.XPDice = ExperienceOf(mod, u).Dice
			o := r.CurrentOrder(u.ID)
			f.Missed = o == nil || o.Status != OrderLocked
			f.OutOfAmmo = outOfAmmo(cl, u)
		}
		s.Fighters = append(s.Fighters, f)
		p := pos[id]
		if p != start[id] {
			s.Holding = false
		}
		s.Cover = max(s.Cover, mod.TerrainAt(r.Map.TerrainCode(p[0], p[1])).Cover)
		// Across a barrier only if every foe it touches is across one.
		for _, fid := range foes {
			fp := pos[fid]
			if HexDistance(p[0], p[1], fp[0], fp[1]) == 1 {
				if side, ok := r.Map.SideBetween(p[0], p[1], fp[0], fp[1]); ok {
					if e := r.Map.EdgeAt(side); e == nil || e.Crossing != "" || !mod.EdgeType(e.Type).Blocks {
						s.Across = false
					}
				}
			} else if p == fp {
				s.Across = false
			}
		}
	}
	if s.Holding {
		s.Across = false // the barrier hurts whoever is trying to cross it
	}
	return s
}

func outOfAmmo(cl *Class, u *Unit) bool {
	if len(cl.Ammo) == 0 {
		return false
	}
	for k := range cl.Ammo {
		if u.ammo(cl, k) > 0 {
			return false
		}
	}
	return true
}

// ammo is the unit's remaining ammunition of one kind; units start full.
func (u *Unit) ammo(cl *Class, kind string) int {
	if n, ok := u.Supply[kind]; ok {
		return n
	}
	return cl.Ammo[kind]
}

// Roll settles an encounter with dice. dice nil means the suggested pools.
// Overriding the pools or entering hits makes it a referee ruling.
func (r *Room) settle(mod *Module, e *Encounter, rng *rand.Rand, dice *[2]int, hits *[2]int, reason string) {
	e.Source = "rule engine"
	if dice != nil {
		if *dice != [2]int{e.Suggested[0].Dice, e.Suggested[1].Dice} {
			e.Source = "referee"
		}
		e.Dice = *dice
	}
	if hits != nil {
		e.Faces, e.Hits, e.Source = [2][]int{}, *hits, "referee"
	} else {
		for side := range 2 {
			e.Faces[side], e.Hits[side] = mod.Combat.Roll(rng, e.Dice[side])
		}
	}
	e.Margin = e.Hits[0] - e.Hits[1]
	switch {
	case e.Margin > 0:
		e.Winner = "friend"
	case e.Margin < 0:
		e.Winner, e.Margin = "enemy", -e.Margin
	default:
		e.Winner = "draw"
	}
	o := mod.Combat.OutcomeFor(e.Margin)
	e.Outcome = &o
	e.Settled = true
	if reason = strings.TrimSpace(reason); reason != "" {
		e.Reason = reason
	}
}
