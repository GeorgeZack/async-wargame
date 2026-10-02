package game

import (
	"fmt"
	"slices"
)

// HexMap is the terrain grid plus linear features. Each hex has one terrain
// code; rivers, ridges, and similar barriers run along hex sides (Edges),
// and roads join hex centers across sides (Roads). A scenario holds the
// template map and each room keeps its own editable copy.
type HexMap struct {
	Cols  int       `json:"cols"`
	Rows  []string  `json:"rows"`
	Edges []Edge    `json:"edges,omitempty"`
	Roads []HexSide `json:"roads,omitempty"`
}

// HexSide identifies one hex side: a hex and a direction (see Directions).
// Every side is shared by two hexes; Canonical picks one spelling for it.
type HexSide struct {
	Col int `json:"col"`
	Row int `json:"row"`
	Dir int `json:"dir"`
}

// Edge is a linear feature along a hex side, optionally with a crossing.
type Edge struct {
	HexSide
	Type     string `json:"type"`               // module edge type: river, stream, ridge, cliff
	Crossing string `json:"crossing,omitempty"` // module crossing: bridge, ford, pass
}

// Directions, in the order of oddRNeighbors.
var Directions = []string{"east", "north-east", "north-west", "west", "south-west", "south-east"}

func (m *HexMap) NumRows() int { return len(m.Rows) }

func (m *HexMap) InBounds(col, row int) bool {
	return row >= 0 && row < len(m.Rows) && col >= 0 && col < m.Cols
}

func (m *HexMap) TerrainCode(col, row int) byte { return m.Rows[row][col] }

func (m *HexMap) SetTerrain(col, row int, code byte) {
	b := []byte(m.Rows[row])
	b[col] = code
	m.Rows[row] = string(b)
}

// Neighbor returns the hex across side dir, and whether it is on the map.
func (m *HexMap) Neighbor(col, row, dir int) (int, int, bool) {
	if dir < 0 || dir > 5 || row < 0 {
		return 0, 0, false
	}
	d := oddRNeighbors[row&1][dir]
	c, r := col+d[0], row+d[1]
	return c, r, m.InBounds(c, r)
}

// Neighbors returns the on-map hexes adjacent to (col, row).
func (m *HexMap) Neighbors(col, row int) [][2]int {
	var out [][2]int
	for dir := range 6 {
		if c, r, ok := m.Neighbor(col, row, dir); ok {
			out = append(out, [2]int{c, r})
		}
	}
	return out
}

// Canonical spells a side from the hex that owns it: east, south-west, and
// south-east sides belong to their hex; the others to the neighbor.
// ok is false for sides on the map's outer boundary.
func (m *HexMap) Canonical(s HexSide) (HexSide, bool) {
	if !m.InBounds(s.Col, s.Row) || s.Dir < 0 || s.Dir > 5 {
		return s, false
	}
	c, r, ok := m.Neighbor(s.Col, s.Row, s.Dir)
	if !ok {
		return s, false
	}
	if s.Dir == 0 || s.Dir == 4 || s.Dir == 5 {
		return s, true
	}
	return HexSide{c, r, (s.Dir + 3) % 6}, true
}

// SideBetween returns the side shared by two adjacent hexes.
func (m *HexMap) SideBetween(c1, r1, c2, r2 int) (HexSide, bool) {
	for dir := range 6 {
		if c, r, ok := m.Neighbor(c1, r1, dir); ok && c == c2 && r == r2 {
			return m.Canonical(HexSide{c1, r1, dir})
		}
	}
	return HexSide{}, false
}

func (m *HexMap) EdgeAt(s HexSide) *Edge {
	cs, ok := m.Canonical(s)
	if !ok {
		return nil
	}
	for i := range m.Edges {
		if m.Edges[i].HexSide == cs {
			return &m.Edges[i]
		}
	}
	return nil
}

func (m *HexMap) HasRoad(s HexSide) bool {
	cs, ok := m.Canonical(s)
	return ok && slices.Contains(m.Roads, cs)
}

// SetEdge sets or (with typ "") removes the feature on a side.
func (m *HexMap) SetEdge(s HexSide, typ, crossing string) error {
	cs, ok := m.Canonical(s)
	if !ok {
		return fmt.Errorf("%w: that side is on the map boundary", ErrBadRequest)
	}
	m.Edges = slices.DeleteFunc(m.Edges, func(e Edge) bool { return e.HexSide == cs })
	if typ != "" {
		m.Edges = append(m.Edges, Edge{HexSide: cs, Type: typ, Crossing: crossing})
	}
	return nil
}

func (m *HexMap) SetRoad(s HexSide, on bool) error {
	cs, ok := m.Canonical(s)
	if !ok {
		return fmt.Errorf("%w: that side is on the map boundary", ErrBadRequest)
	}
	m.Roads = slices.DeleteFunc(m.Roads, func(r HexSide) bool { return r == cs })
	if on {
		m.Roads = append(m.Roads, cs)
	}
	return nil
}

func (m *HexMap) Clone() *HexMap {
	c := &HexMap{Cols: m.Cols, Rows: slices.Clone(m.Rows), Edges: slices.Clone(m.Edges), Roads: slices.Clone(m.Roads)}
	return c
}

// validate checks the map against a module's terrain and edge definitions.
func (m *HexMap) validate(mod *Module) error {
	if len(m.Rows) == 0 || m.Cols <= 0 {
		return fmt.Errorf("map is empty")
	}
	for r, row := range m.Rows {
		if len(row) != m.Cols {
			return fmt.Errorf("map row %d has %d columns, want %d", r, len(row), m.Cols)
		}
		for c := 0; c < len(row); c++ {
			if mod.TerrainAt(row[c]) == nil {
				return fmt.Errorf("map hex %s: unknown terrain %q", HexID(c, r), row[c])
			}
		}
	}
	seen := map[HexSide]bool{}
	for _, e := range m.Edges {
		cs, ok := m.Canonical(e.HexSide)
		if !ok || cs != e.HexSide {
			return fmt.Errorf("edge %s %s is not in canonical form", HexID(e.Col, e.Row), Directions[e.Dir])
		}
		if seen[cs] {
			return fmt.Errorf("two edges on side %s %s", HexID(e.Col, e.Row), Directions[e.Dir])
		}
		seen[cs] = true
		if err := mod.CheckEdge(e.Type, e.Crossing); err != nil {
			return fmt.Errorf("edge %s %s: %w", HexID(e.Col, e.Row), Directions[e.Dir], err)
		}
	}
	for _, s := range m.Roads {
		if cs, ok := m.Canonical(s); !ok || cs != s {
			return fmt.Errorf("road %s %s is not in canonical form", HexID(s.Col, s.Row), Directions[s.Dir])
		}
	}
	return nil
}

// GroundPassable reports whether a ground unit can cross from (col,row) over
// side dir: the far hex must be land and any barrier must have a crossing.
func (m *HexMap) GroundPassable(mod *Module, col, row, dir int) bool {
	c, r, ok := m.Neighbor(col, row, dir)
	if !ok || mod.TerrainAt(m.TerrainCode(c, r)).Water() {
		return false
	}
	if e := m.EdgeAt(HexSide{col, row, dir}); e != nil {
		if et := mod.EdgeType(e.Type); et != nil && et.Blocks && e.Crossing == "" {
			return false
		}
	}
	return true
}

// ParseHexID turns "0712" back into zero-based (col, row).
func ParseHexID(id string) (col, row int, ok bool) {
	if len(id) != 4 {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(id, "%02d%02d", &col, &row); err != nil {
		return 0, 0, false
	}
	return col - 1, row - 1, true
}
