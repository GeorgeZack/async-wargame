package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// MapView is precomputed SVG geometry for a pointy-top, odd-row-offset hex grid.
type MapView struct {
	ViewBox         string
	Width           float64
	Height          float64
	HexPoints       string
	Cols            int
	Rows            int
	Cells           []Cell
	Coast           string     // path data along every sea/land hex side
	EdgePaths       []EdgePath // one path per edge type (river, ridge, ...)
	Crossings       []CrossingMark
	Roads           string // path data joining hex centers
	SectorLines     string // path data along sector boundaries
	Fog             string // path data covering hexes no friendly unit can see
	SectorLabels    []SectorLabel
	Features        []FeatureMark
	Stacks          []Stack
	Proposed        []ProposedMark // referee only: hexes a pending event would change
	OrderLines      []OrderLine
	ResolutionLines []ResolutionLine // referee: this turn\'s moves
	EncounterMarks  []EncounterMark
}

type Cell struct {
	X, Y  float64
	T     string // terrain id
	Hex   string
	Depth int // water only: distance to the nearest land, capped at 3
}

type SectorLabel struct {
	X, Y     float64
	ID, Name string
}

type EdgePath struct {
	Type string
	D    string
}

// CrossingMark is drawn across a side, along the line between hex centers.
type CrossingMark struct {
	X1, Y1, X2, Y2 float64
	Type           string
	Label          string
}

type ResolutionLine struct {
	Points, Role, Label string
}

type EncounterMark struct {
	X, Y    float64
	Label   string
	Settled bool
}

type ProposedMark struct {
	X, Y  float64
	Hex   string
	Label string
}

type FeatureMark struct {
	X, Y  float64
	Glyph string
	FeatureView
}

type Stack struct {
	X, Y  float64
	Hex   string
	Top   UnitView
	Count int
	Label string // accessible description of the stack
	Full  bool   // friendly stack at its limit
}

// InspectorData is the JSON the client uses to describe a selected hex.
type InspectorData struct {
	Cols     int                      `json:"cols"`
	Rows     int                      `json:"rows"`
	Terrain  map[string]Terrain       `json:"terrain"`
	Units    map[string][]UnitView    `json:"units"`
	Features map[string][]FeatureView `json:"features"`
	Sides    map[string][]SideInfo    `json:"sides"` // edges and roads, listed from each hex's point of view
	Sectors  SectorGrid               `json:"sectors"`
	Orders   OrderData                `json:"orders"`
	Stack    map[string][2]int        `json:"stack"` // hex -> [friendly stacking points used, limit]
}

type SideInfo struct {
	Dir       int    `json:"dir"`
	DirName   string `json:"dirName"`
	Edge      string `json:"edge,omitempty"`
	EdgeName  string `json:"edgeName,omitempty"`
	Crossing  string `json:"crossing,omitempty"`
	CrossName string `json:"crossingName,omitempty"`
	Blocks    bool   `json:"blocks,omitempty"` // impassable to ground units here
	Road      bool   `json:"road,omitempty"`
	Note      string `json:"note,omitempty"`
}

const hexSize = 18.0

var hexW = math.Sqrt(3) * hexSize

// HexCenter returns the SVG coordinates of a hex center.
func HexCenter(col, row int) (x, y float64) {
	x = hexW*(float64(col)+0.5*float64(row&1)) + hexW/2
	y = 1.5*hexSize*float64(row) + hexSize
	return
}

// sideEnds returns the two corners of a hex side. Side d runs between the
// corners at 30-60d and -30-60d degrees (screen coordinates, y down).
func sideEnds(col, row, dir int) (x1, y1, x2, y2 float64) {
	cx, cy := HexCenter(col, row)
	a1 := (30 - 60*float64(dir)) * math.Pi / 180
	a2 := (-30 - 60*float64(dir)) * math.Pi / 180
	return cx + hexSize*math.Cos(a1), cy + hexSize*math.Sin(a1), cx + hexSize*math.Cos(a2), cy + hexSize*math.Sin(a2)
}

// dirVector is the unit vector from a hex center toward its neighbor across side dir.
func dirVector(dir int) (float64, float64) {
	a := -60 * float64(dir) * math.Pi / 180
	return math.Cos(a), math.Sin(a)
}

var featureGlyphs = map[string]string{
	"beachhead": "B", "hospital": "+", "port": "P", "utility": "U",
	"police": "S", "shelter": "C", "relay": "R", "hive": "!",
}

// FeatureKinds lists the place kinds the editor offers.
var FeatureKinds = []string{"beachhead", "hospital", "port", "utility", "police", "shelter", "relay", "hive"}

func buildMap(r *Room, mod *Module, ref bool, units []UnitView, local []observer) (MapView, []Terrain, InspectorData) {
	m := r.Map
	rows := m.NumRows()
	w := hexW * (float64(m.Cols) + 0.5)
	h := 1.5*hexSize*float64(rows-1) + 2*hexSize
	mv := MapView{
		ViewBox: fmt.Sprintf("0 0 %.1f %.1f", w, h), Width: w, Height: h, Cols: m.Cols, Rows: rows,
		HexPoints: fmt.Sprintf("0,%.1f %.2f,%.1f %.2f,%.1f 0,%.1f %.2f,%.1f %.2f,%.1f",
			-hexSize, hexW/2, -hexSize/2, hexW/2, hexSize/2, hexSize, -hexW/2, hexSize/2, -hexW/2, -hexSize/2),
	}
	data := InspectorData{Cols: m.Cols, Rows: rows, Terrain: map[string]Terrain{},
		Units: map[string][]UnitView{}, Features: map[string][]FeatureView{}, Sides: map[string][]SideInfo{}}

	water := func(c, r int) bool { return mod.TerrainAt(m.TerrainCode(c, r)).Water() }
	depth := waterDepth(m, water)
	used := map[string]bool{}
	var coast strings.Builder
	for row := range rows {
		for col := 0; col < m.Cols; col++ {
			t := mod.TerrainAt(m.TerrainCode(col, row))
			x, y := HexCenter(col, row)
			mv.Cells = append(mv.Cells, Cell{X: x, Y: y, T: t.ID, Hex: HexID(col, row), Depth: depth[row][col]})
			used[t.ID] = true
			// Coastline: owned sides between water and land.
			for _, dir := range []int{0, 4, 5} {
				if nc, nr, ok := m.Neighbor(col, row, dir); ok && water(col, row) != water(nc, nr) {
					x1, y1, x2, y2 := sideEnds(col, row, dir)
					fmt.Fprintf(&coast, "M%.1f %.1fL%.1f %.1f", x1, y1, x2, y2)
				}
			}
		}
	}
	mv.Coast = coast.String()

	observed := r.observedBy(append(r.observers(mod), local...))
	var fog strings.Builder
	for row := range rows {
		for col := 0; col < m.Cols; col++ {
			if observed[[2]int{col, row}] {
				continue
			}
			x, y := HexCenter(col, row)
			fmt.Fprintf(&fog, "M%.1f %.1fl%.2f %.1fv%.1fl%.2f %.1fl%.2f %.1fv%.1fz",
				x, y-hexSize, hexW/2, hexSize/2, hexSize, -hexW/2, hexSize/2, -hexW/2, -hexSize/2, -hexSize)
		}
	}
	mv.Fog = fog.String()

	// Sector boundaries run along hex sides between blocks.
	var sectors strings.Builder
	for row := range rows {
		for col := 0; col < m.Cols; col++ {
			for _, dir := range []int{0, 4, 5} {
				if nc, nr, ok := m.Neighbor(col, row, dir); ok && m.SectorOf(col, row) != m.SectorOf(nc, nr) {
					x1, y1, x2, y2 := sideEnds(col, row, dir)
					fmt.Fprintf(&sectors, "M%.1f %.1fL%.1f %.1f", x1, y1, x2, y2)
				}
			}
		}
	}
	mv.SectorLines = sectors.String()
	g := m.sectors()
	data.Sectors = g
	for row := 0; row < rows; row += g.Rows {
		for col := 0; col < m.Cols; col += g.Cols {
			x, y := HexCenter(col, row)
			id := m.SectorOf(col, row)
			mv.SectorLabels = append(mv.SectorLabels, SectorLabel{X: x - hexW/2 + 2, Y: y - 4, ID: id, Name: g.Names[id]})
		}
	}

	var legend []Terrain
	for _, t := range mod.Terrain {
		data.Terrain[t.ID] = t
		if used[t.ID] {
			legend = append(legend, t)
		}
	}

	// Edges, drawn in module order so rivers sit under ridges consistently.
	paths := map[string]*strings.Builder{}
	sides := map[string]map[int]*SideInfo{}
	side := func(col, row, dir int) *SideInfo {
		hx := HexID(col, row)
		if sides[hx] == nil {
			sides[hx] = map[int]*SideInfo{}
		}
		if sides[hx][dir] == nil {
			sides[hx][dir] = &SideInfo{Dir: dir, DirName: Directions[dir]}
		}
		return sides[hx][dir]
	}
	for _, e := range m.Edges {
		et := mod.EdgeType(e.Type)
		if et == nil {
			continue
		}
		if paths[e.Type] == nil {
			paths[e.Type] = &strings.Builder{}
		}
		x1, y1, x2, y2 := sideEnds(e.Col, e.Row, e.Dir)
		fmt.Fprintf(paths[e.Type], "M%.1f %.1fL%.1f %.1f", x1, y1, x2, y2)
		cross := mod.Crossing(e.Crossing)
		if cross != nil {
			ux, uy := dirVector(e.Dir)
			cx, cy := HexCenter(e.Col, e.Row)
			a, b := hexW/2-5.5, hexW/2+5.5
			mv.Crossings = append(mv.Crossings, CrossingMark{X1: cx + ux*a, Y1: cy + uy*a, X2: cx + ux*b, Y2: cy + uy*b,
				Type: cross.ID, Label: fmt.Sprintf("%s over the %s", cross.Name, strings.ToLower(et.Name))})
		}
		nc, nr, _ := m.Neighbor(e.Col, e.Row, e.Dir)
		for _, s := range []*SideInfo{side(e.Col, e.Row, e.Dir), side(nc, nr, (e.Dir+3)%6)} {
			s.Edge, s.EdgeName, s.Blocks, s.Note = et.ID, et.Name, et.Blocks && cross == nil, et.Note
			if cross != nil {
				s.Crossing, s.CrossName, s.Note = cross.ID, cross.Name, cross.Note
			}
		}
	}
	for _, et := range mod.EdgeTypes {
		if p := paths[et.ID]; p != nil {
			mv.EdgePaths = append(mv.EdgePaths, EdgePath{Type: et.ID, D: p.String()})
		}
	}

	var roads strings.Builder
	for _, s := range m.Roads {
		nc, nr, ok := m.Neighbor(s.Col, s.Row, s.Dir)
		if !ok {
			continue
		}
		x1, y1 := HexCenter(s.Col, s.Row)
		x2, y2 := HexCenter(nc, nr)
		fmt.Fprintf(&roads, "M%.1f %.1fL%.1f %.1f", x1, y1, x2, y2)
		side(s.Col, s.Row, s.Dir).Road = true
		side(nc, nr, (s.Dir+3)%6).Road = true
	}
	mv.Roads = roads.String()
	for hx, byDir := range sides {
		for dir := range 6 {
			if s := byDir[dir]; s != nil {
				data.Sides[hx] = append(data.Sides[hx], *s)
			}
		}
	}

	for _, f := range r.Features {
		if f.Hidden && !ref {
			continue
		}
		x, y := HexCenter(f.Col, f.Row)
		fv := FeatureView{Name: f.Name, Kind: f.Kind, Hex: HexID(f.Col, f.Row), Note: f.Note, Hidden: f.Hidden}
		if ref {
			fv.ID = f.ID
		}
		mv.Features = append(mv.Features, FeatureMark{X: x, Y: y, Glyph: featureGlyphs[f.Kind], FeatureView: fv})
		data.Features[fv.Hex] = append(data.Features[fv.Hex], fv)
	}

	// Stack order: the viewer's own units on top, then friendly, then the rest.
	byHex := map[string][]UnitView{}
	var order []string
	for _, u := range units {
		if byHex[u.Hex] == nil {
			order = append(order, u.Hex)
		}
		byHex[u.Hex] = append(byHex[u.Hex], u)
	}
	data.Stack = map[string][2]int{}
	load := r.Load(mod)
	for h, used := range load {
		if used > 0 {
			data.Stack[HexID(h[0], h[1])] = [2]int{used, r.StackLimit(mod, h[0], h[1])}
		}
	}
	for _, hx := range order {
		us := byHex[hx]
		sort.SliceStable(us, func(i, j int) bool { return stackRank(us[i]) < stackRank(us[j]) })
		data.Units[hx] = us
		col, row, _ := ParseHexID(hx)
		x, y := HexCenter(col, row)
		names := make([]string, len(us))
		for i, u := range us {
			names[i] = u.Name
			switch {
			case u.Stale:
				names[i] += fmt.Sprintf(" (off the net: last reported here, turn %d)", u.SeenTurn)
			case u.LocalSight:
				names[i] += " (seen by your unit off the net)"
			}
		}
		st := data.Stack[hx]
		mv.Stacks = append(mv.Stacks, Stack{X: x, Y: y, Hex: hx, Top: us[0], Count: len(us), Full: st[1] > 0 && st[0] >= st[1],
			Label: fmt.Sprintf("Hex %s: %s", hx, strings.Join(names, ", "))})
	}
	return mv, legend, data
}

// waterDepth gives each water hex its distance to land (1 = shore), capped at 3.
func waterDepth(m *HexMap, water func(c, r int) bool) [][]int {
	d := make([][]int, m.NumRows())
	var queue [][2]int
	for r := range d {
		d[r] = make([]int, m.Cols)
		for c := range d[r] {
			if water(c, r) {
				d[r][c] = 3
				for _, n := range m.Neighbors(c, r) {
					if !water(n[0], n[1]) {
						d[r][c] = 1
						queue = append(queue, [2]int{c, r})
						break
					}
				}
			}
		}
	}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		for _, n := range m.Neighbors(h[0], h[1]) {
			if nd := d[h[1]][h[0]] + 1; water(n[0], n[1]) && nd < d[n[1]][n[0]] {
				d[n[1]][n[0]] = nd
				queue = append(queue, n)
			}
		}
	}
	return d
}
