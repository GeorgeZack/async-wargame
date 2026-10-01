package game

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Fog of war. Players share one picture: whatever any friendly unit can see
// right now, plus what they've been told about (Known units, such as a
// contact a drone is tracking). Contacts that slip out of sight leave a
// "last seen" marker where they were. The referee sees everything.
//
// There's no line-of-sight tracing yet: sight is a range, shortened by cover
// at the target. That's enough for buildings and woods to hide the dead.

// Sighting is where players last saw an enemy unit.
type Sighting struct {
	Col  int       `json:"col"`
	Row  int       `json:"row"`
	Turn int       `json:"turn"`
	At   time.Time `json:"at"`
	Live bool      `json:"live"` // in sight right now
}

// sightRange is how far a friendly unit sees, from the module's sight rules.
func (r *Room) sightRange(mod *Module, u *Unit) int {
	return mod.Sight.RangeFor(mod.Class(u.Class), mod.TerrainAt(r.Map.TerrainCode(u.Col, u.Row)))
}

// concealment caps the range at which a unit in this hex can be seen.
func (r *Room) concealment(mod *Module, u *Unit, col, row int) int {
	return mod.Sight.Concealment(mod.Class(u.Class), mod.TerrainAt(r.Map.TerrainCode(col, row)))
}

type observer struct{ col, row, rng int }

func (r *Room) observers(mod *Module) []observer {
	var out []observer
	for _, u := range r.Units {
		if s := mod.Side(u.Side); s != nil && s.Playable {
			out = append(out, observer{u.Col, u.Row, r.sightRange(mod, u)})
		}
	}
	return out
}

// Observed is the set of hexes some friendly unit can see into.
func (r *Room) Observed(mod *Module) map[[2]int]bool {
	seen := map[[2]int]bool{}
	obs := r.observers(mod)
	for row := range r.Map.NumRows() {
		for col := 0; col < r.Map.Cols; col++ {
			for _, o := range obs {
				if HexDistance(o.col, o.row, col, row) <= o.rng {
					seen[[2]int{col, row}] = true
					break
				}
			}
		}
	}
	return seen
}

// inSight reports whether friendly units can see unit u where it stands.
func (r *Room) inSight(mod *Module, u *Unit, obs []observer) bool {
	limit := r.concealment(mod, u, u.Col, u.Row)
	for _, o := range obs {
		if HexDistance(o.col, o.row, u.Col, u.Row) <= min(o.rng, limit) {
			return true
		}
	}
	return false
}

// VisibleToPlayers reports whether players currently see u.
func (r *Room) VisibleToPlayers(mod *Module, u *Unit) bool {
	if s := mod.Side(u.Side); s.Playable || u.Known {
		return true
	}
	sg, ok := r.Sightings[u.ID]
	return ok && sg.Live && sg.Col == u.Col && sg.Row == u.Row
}

// RefreshFog updates sightings after any change, and reports new contacts
// to players. The store calls it after every update.
func (r *Room) RefreshFog(mod *Module, now time.Time) {
	if r.Sightings == nil {
		r.Sightings = map[string]Sighting{}
	}
	obs := r.observers(mod)
	present := map[string]bool{}
	var spotted, found []string
	for _, u := range r.Units {
		side := mod.Side(u.Side)
		if side.Playable {
			continue
		}
		present[u.ID] = true
		seen := r.inSight(mod, u, obs)
		if !side.Enemy {
			if seen && !u.Known { // civilians, once found, stay on the map
				u.Known = true
				found = append(found, fmt.Sprintf("%s at %s (%s)", u.Name, HexID(u.Col, u.Row), r.Map.SectorAt(u.Col, u.Row)))
			}
			continue
		}
		prev, had := r.Sightings[u.ID]
		switch {
		case seen:
			if !had || !prev.Live {
				spotted = append(spotted, r.Map.SectorAt(u.Col, u.Row))
			}
			r.Sightings[u.ID] = Sighting{Col: u.Col, Row: u.Row, Turn: r.Turn, At: now.UTC(), Live: true}
		case had && prev.Live:
			prev.Live = false
			r.Sightings[u.ID] = prev
		}
	}
	for id := range r.Sightings {
		if !present[id] {
			delete(r.Sightings, id)
		}
	}
	if r.Phase == PhaseSetup {
		return // the scenario is still being built; no reports
	}
	if len(spotted) > 0 {
		r.logKind(VisAll, "sitrep", "%s in %s.", capitalize(plural(len(spotted), "new contact", "new contacts")), sectorList(spotted))
	}
	for _, f := range found {
		r.logKind(VisAll, "sitrep", "Survivors found: %s.", f)
	}
}

func capitalize(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func sectorList(labels []string) string {
	count := map[string]int{}
	for _, l := range labels {
		count[l]++
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if count[k] > 1 {
			parts = append(parts, fmt.Sprintf("sector %s (%d)", k, count[k]))
		} else {
			parts = append(parts, "sector "+k)
		}
	}
	return joinList(parts)
}
