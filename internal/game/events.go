package game

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Random events are declarative module data: a weighted table whose entries
// are built from a small, fixed set of effect operations. Nothing in the
// table is executed as code.
//
// Rolling turns a definition into a concrete proposal (which hexes, how many,
// which units) using a recorded seed. The proposal changes nothing until the
// referee applies it, and both the proposal and the decision stay in the
// campaign record.

const (
	MinThreat        = 1
	MaxThreat        = 5
	maxOpenProposals = 6
)

type EventStatus string

const (
	EventProposed  EventStatus = "proposed"
	EventApplied   EventStatus = "applied"
	EventDiscarded EventStatus = "discarded"
)

// GameEvent is a rolled event: a concrete, reviewable proposal and its fate.
type GameEvent struct {
	ID        string      `json:"id"`
	Def       string      `json:"def"`
	Title     string      `json:"title"`
	Category  string      `json:"category"`
	Status    EventStatus `json:"status"`
	Chosen    bool        `json:"chosen,omitempty"` // picked by the referee rather than rolled
	Roll      int         `json:"roll,omitempty"`
	Of        int         `json:"of,omitempty"`
	Seed      uint64      `json:"seed"`
	Threat    int         `json:"threat"`
	Turn      int         `json:"turn"`
	At        time.Time   `json:"at"`
	DecidedAt *time.Time  `json:"decidedAt,omitempty"`
	Summary   string      `json:"summary"`
	Report    string      `json:"report,omitempty"`
	Effects   []Effect    `json:"effects"`
}

// Effect is one concrete state change in a proposal.
type Effect struct {
	Op       string `json:"op"`
	UnitID   string `json:"unit,omitempty"`
	Class    string `json:"class,omitempty"`
	Col      int    `json:"col"`
	Row      int    `json:"row"`
	ToCol    int    `json:"toCol,omitempty"`
	ToRow    int    `json:"toRow,omitempty"`
	Strength int    `json:"strength,omitempty"`
	Delta    int    `json:"delta,omitempty"`
	Known    bool   `json:"known,omitempty"`
	Text     string `json:"text"`
}

// countPattern matches report placeholders {n} and {n:noun}, which become
// the number of affected hexes, with the noun pluralized to match.
var countPattern = regexp.MustCompile(`\{n(?::(\w+))?\}`)

func (e Effect) located() bool { return e.Op != "threat" }

func threatFactor(threat int) float64 { return 1 + 0.25*float64(threat-1) }

func newRNG() (*rand.Rand, uint64) {
	var b [8]byte
	crand.Read(b[:])
	seed := binary.LittleEndian.Uint64(b[:])
	return rngFor(seed), seed
}

func rngFor(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

func (r *Room) hasClass(class string) bool {
	return slices.ContainsFunc(r.Units, func(u *Unit) bool { return u.Class == class })
}

// Eligible reports whether an event can come up in a random roll right now.
func (r *Room) Eligible(e *EventDef) bool { return r.eligibleAt(e, r.Threat) }

func (r *Room) eligibleAt(e *EventDef, threat int) bool {
	switch {
	case e.MinThreat > 0 && threat < e.MinThreat,
		e.MaxThreat > 0 && threat > e.MaxThreat,
		e.Requires != "" && !r.hasClass(e.Requires),
		e.Unique != "" && r.hasClass(e.Unique):
		return false
	}
	return true
}

func (r *Room) OpenProposals() int {
	n := 0
	for _, e := range r.Events {
		if e.Status == EventProposed {
			n++
		}
	}
	return n
}

// RollEvent draws from the module's event table, or uses defID when the
// referee picks one, and records a proposal.
func (r *Room) RollEvent(actor *Member, mod *Module, defID string) (*GameEvent, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	if r.OpenProposals() >= maxOpenProposals {
		return nil, fmt.Errorf("%w: apply or discard some proposed events first", ErrBadRequest)
	}
	rng, seed := newRNG()
	var def *EventDef
	roll, of := 0, 0
	if defID != "" {
		if def = mod.Event(defID); def == nil {
			return nil, ErrNotFound
		}
	} else {
		var pool []*EventDef
		for i := range mod.Events {
			if d := &mod.Events[i]; r.Eligible(d) {
				pool = append(pool, d)
				of += d.Weight
			}
		}
		if of == 0 {
			return nil, fmt.Errorf("%w: no events are possible at this threat level", ErrBadRequest)
		}
		roll = rng.IntN(of) + 1
		for acc, i := 0, 0; ; i++ {
			if acc += pool[i].Weight; roll <= acc {
				def = pool[i]
				break
			}
		}
	}
	ev := r.propose(def, rng, mod)
	ev.Seed, ev.Roll, ev.Of, ev.Chosen = seed, roll, of, defID != ""
	r.Events = append(r.Events, ev)
	return ev, nil
}

// propose resolves every effect of def against the current state.
func (r *Room) propose(def *EventDef, rng *rand.Rand, mod *Module) *GameEvent {
	ev := &GameEvent{
		ID: newID(), Def: def.ID, Title: def.Title, Category: def.Category, Status: EventProposed,
		Threat: r.Threat, Turn: r.Turn, At: time.Now().UTC(), Summary: def.Referee,
	}
	marines := r.marinePositions(mod)
	taken := map[[2]int]bool{}
	for _, fd := range def.Effects {
		switch fd.Op {
		case "spawn":
			ev.Effects = append(ev.Effects, r.proposeSpawn(fd, rng, mod, marines, taken)...)
		case "grow":
			ev.Effects = append(ev.Effects, r.proposeGrow(fd, rng)...)
		case "drift":
			ev.Effects = append(ev.Effects, r.proposeDrift(fd, rng, mod, marines)...)
		case "reveal":
			ev.Effects = append(ev.Effects, r.proposeReveal(fd, rng, marines)...)
		case "jam":
			ev.Effects = append(ev.Effects, r.proposeJam(fd, rng, mod)...)
		case "threat":
			ev.Effects = append(ev.Effects, Effect{Op: "threat", Delta: fd.Amount,
				Text: fmt.Sprintf("Threat level %+d", fd.Amount)})
		}
	}

	located := 0
	var first *Effect
	for i := range ev.Effects {
		if ev.Effects[i].located() {
			located++
			if first == nil {
				first = &ev.Effects[i]
			}
		}
	}
	switch {
	case len(def.Effects) > 0 && located == 0:
		ev.Summary += " (No valid targets right now, so this has no effect.)"
	case def.Report != "":
		dir := "inland"
		if first != nil {
			c, rw := centroid(marines, r.Map)
			dir = Compass(c, rw, first.Col, first.Row)
		}
		report := strings.ReplaceAll(def.Report, "{dir}", dir)
		ev.Report = countPattern.ReplaceAllStringFunc(report, func(m string) string {
			if noun := countPattern.FindStringSubmatch(m)[1]; noun != "" {
				return plural(located, noun, noun+"s")
			}
			return fmt.Sprint(located)
		})
	}
	return ev
}

func (r *Room) marinePositions(mod *Module) [][2]int {
	var out [][2]int
	for _, u := range r.Units {
		if s := mod.Side(u.Side); s != nil && s.Playable && mod.Class(u.Class).Domain == "land" {
			out = append(out, [2]int{u.Col, u.Row})
		}
	}
	return out
}

func centroid(pts [][2]int, m *HexMap) (int, int) {
	if len(pts) == 0 {
		return m.Cols / 2, m.NumRows() / 2
	}
	var c, r int
	for _, p := range pts {
		c += p[0]
		r += p[1]
	}
	return c / len(pts), r / len(pts)
}

func nearest(col, row int, pts [][2]int) (best [2]int, dist int) {
	dist = 1 << 30
	for _, p := range pts {
		if d := HexDistance(col, row, p[0], p[1]); d < dist {
			best, dist = p, d
		}
	}
	return
}

func (r *Room) proposeSpawn(fd EffectDef, rng *rand.Rand, mod *Module, marines [][2]int, taken map[[2]int]bool) []Effect {
	m := r.Map
	cl := mod.Class(fd.Class)
	w := fd.Where
	var cands [][2]int
	for row := range m.Rows {
		for col := 0; col < m.Cols; col++ {
			t := mod.TerrainAt(m.TerrainCode(col, row))
			if t.Water() || taken[[2]int{col, row}] {
				continue
			}
			if _, d := nearest(col, row, marines); len(marines) > 0 && d < max(1, w.AwayFromMarines) {
				continue
			}
			if w.Edge != "" && !onEdge(w.Edge, col, row, m) {
				continue
			}
			if len(w.Terrain) > 0 && !slices.Contains(w.Terrain, t.ID) {
				continue
			}
			if w.NearFeature != "" && !slices.ContainsFunc(r.Features, func(f Feature) bool {
				return f.Kind == w.NearFeature && HexDistance(col, row, f.Col, f.Row) <= 1
			}) {
				continue
			}
			if w.NearClass != "" && !slices.ContainsFunc(r.Units, func(u *Unit) bool {
				return u.Class == w.NearClass && HexDistance(col, row, u.Col, u.Row) <= 1
			}) {
				continue
			}
			cands = append(cands, [2]int{col, row})
		}
	}
	n := fd.Count.Draw(rng)
	var out []Effect
	for i := 0; i < n && len(cands) > 0; i++ {
		k := rng.IntN(len(cands))
		h := cands[k]
		cands = slices.Delete(cands, k, k+1)
		taken[h] = true
		str := max(1, fd.Strength.Draw(rng))
		if fd.Scale {
			str = max(1, int(float64(str)*threatFactor(r.Threat)+0.5))
		}
		size := ""
		if cl.IsGroup() {
			size = fmt.Sprintf(" (%d)", str)
		}
		vis := "hidden from players"
		if fd.Known {
			vis = "seen by players"
		}
		out = append(out, Effect{Op: "spawn", Class: cl.ID, Col: h[0], Row: h[1], Strength: str, Known: fd.Known,
			Text: fmt.Sprintf("New %s%s at %s, %s", cl.Name, size, HexID(h[0], h[1]), vis)})
	}
	return out
}

func onEdge(edge string, col, row int, m *HexMap) bool {
	north, south := row <= 1, row >= m.NumRows()-2
	west, east := col <= 1, col >= m.Cols-2
	switch edge {
	case "north":
		return north
	case "south":
		return south
	case "east":
		return east
	case "west":
		return west
	case "inland": // the sea is west on this map; any other edge
		return north || south || east
	}
	return false
}

// pick returns Count random matches, or all of them when Count is 0..0.
func pick[T any](rng *rand.Rand, items []T, count Range) []T {
	n := count.Draw(rng)
	if count == (Range{}) || n >= len(items) {
		return items
	}
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return items[:n]
}

func (r *Room) unitsOf(classes []string) []*Unit {
	var out []*Unit
	for _, u := range r.Units {
		if slices.Contains(classes, u.Class) {
			out = append(out, u)
		}
	}
	return out
}

func (r *Room) proposeGrow(fd EffectDef, rng *rand.Rand) []Effect {
	var out []Effect
	for _, u := range pick(rng, r.unitsOf(fd.Classes), fd.Count) {
		pct := float64(fd.Percent.Draw(rng))
		if fd.Scale {
			pct *= threatFactor(r.Threat)
		}
		d := max(1, int(float64(u.Strength)*pct/100+0.5))
		out = append(out, Effect{Op: "grow", UnitID: u.ID, Col: u.Col, Row: u.Row, Delta: d,
			Text: fmt.Sprintf("%s grows %d → %d", u.Name, u.Strength, u.Strength+d)})
	}
	return out
}

func (r *Room) proposeDrift(fd EffectDef, rng *rand.Rand, mod *Module, marines [][2]int) []Effect {
	var targets [][2]int
	if fd.Toward == "players" {
		targets = marines
	} else {
		for _, f := range r.Features {
			if f.Kind == fd.Toward {
				targets = append(targets, [2]int{f.Col, f.Row})
			}
		}
	}
	if len(targets) == 0 {
		return nil
	}
	occupied := map[[2]int]bool{}
	for _, m := range marines {
		occupied[m] = true
	}
	var movers []*Unit
	for _, u := range r.unitsOf(fd.Classes) {
		if mod.Class(u.Class).Movement > 0 {
			movers = append(movers, u)
		}
	}
	var out []Effect
	for _, u := range pick(rng, movers, fd.Count) {
		col, row := u.Col, u.Row
		goal, dist := nearest(col, row, targets)
		for s := fd.Steps.Draw(rng); s > 0; s-- {
			best, bestD := [2]int{-1, -1}, dist
			for dir := range 6 { // rivers, ridges, and cliffs channel the dead to crossings
				nc, nr, _ := r.Map.Neighbor(col, row, dir)
				n := [2]int{nc, nr}
				if !r.Map.GroundPassable(mod, col, row, dir) || occupied[n] {
					continue
				}
				if d := HexDistance(n[0], n[1], goal[0], goal[1]); d < bestD {
					best, bestD = n, d
				}
			}
			if best[0] < 0 {
				break
			}
			col, row, dist = best[0], best[1], bestD
		}
		if col == u.Col && row == u.Row {
			continue
		}
		out = append(out, Effect{Op: "drift", UnitID: u.ID, Col: u.Col, Row: u.Row, ToCol: col, ToRow: row,
			Text: fmt.Sprintf("%s moves %s → %s", u.Name, HexID(u.Col, u.Row), HexID(col, row))})
	}
	return out
}

func (r *Room) proposeReveal(fd EffectDef, rng *rand.Rand, marines [][2]int) []Effect {
	var hidden []*Unit
	for _, u := range r.Units {
		if u.Side != fd.Side || u.Known {
			continue
		}
		if _, d := nearest(u.Col, u.Row, marines); fd.Within > 0 && d > fd.Within {
			continue
		}
		hidden = append(hidden, u)
	}
	if fd.Within > 0 { // closest first
		slices.SortStableFunc(hidden, func(a, b *Unit) int {
			_, da := nearest(a.Col, a.Row, marines)
			_, db := nearest(b.Col, b.Row, marines)
			return da - db
		})
		if n := fd.Count.Draw(rng); n < len(hidden) {
			hidden = hidden[:n]
		}
	} else {
		hidden = pick(rng, hidden, fd.Count)
	}
	var out []Effect
	for _, u := range hidden {
		out = append(out, Effect{Op: "reveal", UnitID: u.ID, Col: u.Col, Row: u.Row,
			Text: fmt.Sprintf("Players learn of %s at %s", u.Name, HexID(u.Col, u.Row))})
	}
	return out
}

func (r *Room) proposal(actor *Member, id string) (*GameEvent, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	for _, e := range r.Events {
		if e.ID == id {
			if e.Status != EventProposed {
				return nil, fmt.Errorf("%w: that event was already %s", ErrBadRequest, e.Status)
			}
			return e, nil
		}
	}
	return nil, ErrNotFound
}

// ApplyEvent makes a proposal canonical. Effects whose targets have changed
// since the roll (a unit moved or is gone) are skipped and noted.
func (r *Room) ApplyEvent(actor *Member, mod *Module, id string) error {
	ev, err := r.proposal(actor, id)
	if err != nil {
		return err
	}
	r.applyEvent(mod, ev, time.Now())
	return nil
}

func (r *Room) applyEvent(mod *Module, ev *GameEvent, now time.Time) {
	var done, skipped []string
	for _, e := range ev.Effects {
		u := r.Unit(e.UnitID)
		switch e.Op {
		case "spawn":
			cl := mod.Class(e.Class)
			r.Seq++
			n := &Unit{ID: fmt.Sprintf("s%d", r.Seq), Name: fmt.Sprintf("%s %d", cl.Name, r.Seq), Class: cl.ID,
				Side: cl.Side, Col: e.Col, Row: e.Row, Strength: e.Strength, Known: e.Known}
			r.Units = append(r.Units, n)
			done = append(done, strings.Replace(e.Text, "New "+cl.Name, n.Name, 1))
			continue
		case "grow":
			if u != nil {
				u.Strength += e.Delta
			}
		case "drift":
			if u != nil && u.Col == e.Col && u.Row == e.Row {
				u.Col, u.Row = e.ToCol, e.ToRow
			} else {
				u = nil
			}
		case "reveal":
			if u != nil {
				u.Known = true
			}
		case "jam":
			if u != nil {
				u.JammedUntil = max(u.JammedUntil, r.Turn+e.Delta-1)
			}
		case "contact", "note": // for the referee; no state change
			done = append(done, e.Text)
			continue
		case "threat":
			r.Threat = min(MaxThreat, max(MinThreat, r.Threat+e.Delta))
			done = append(done, fmt.Sprintf("Threat level is now %d", r.Threat))
			continue
		}
		if u == nil {
			skipped = append(skipped, e.Text)
		} else {
			done = append(done, e.Text)
		}
	}
	now = now.UTC()
	ev.Status, ev.DecidedAt = EventApplied, &now
	detail := strings.Join(done, "; ")
	if len(skipped) > 0 {
		detail += fmt.Sprintf(" (skipped, state changed since the roll: %s)", strings.Join(skipped, "; "))
	}
	if detail == "" {
		detail = "no state changes"
	}
	r.logKind(VisReferee, "event", "Event applied: %s. %s.", ev.Title, detail)
	if ev.Report != "" {
		r.logKind(VisAll, "sitrep", "%s", ev.Report)
	}

}

func (r *Room) DiscardEvent(actor *Member, id string) error {
	ev, err := r.proposal(actor, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	ev.Status, ev.DecidedAt = EventDiscarded, &now
	r.logKind(VisReferee, "event", "Event discarded: %s.", ev.Title)
	return nil
}

func (r *Room) SetThreat(actor *Member, delta int) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	t := min(MaxThreat, max(MinThreat, r.Threat+delta))
	if t != r.Threat {
		r.Threat = t
		r.logKind(VisReferee, "event", "Threat level set to %d.", t)
	}
	return nil
}

// proposeJam picks friendly units whose radios go out for a few turns.
func (r *Room) proposeJam(fd EffectDef, rng *rand.Rand, mod *Module) []Effect {
	var friendly []*Unit
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable && mod.Class(u.Class).Domain == "land" {
			friendly = append(friendly, u)
		}
	}
	var out []Effect
	for _, u := range pick(rng, friendly, fd.Count) {
		turns := max(1, fd.Turns.Draw(rng))
		out = append(out, Effect{Op: "jam", UnitID: u.ID, Col: u.Col, Row: u.Row, Delta: turns,
			Text: fmt.Sprintf("%s's radio is jammed for %s", u.Name, plural(turns, "turn", "turns"))})
	}
	return out
}
