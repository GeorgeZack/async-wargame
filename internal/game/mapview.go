package game

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// MapView is the board as SVG geometry: the background (the drawn hex map,
// or an image), a grid overlay, places, and markers at their exact points.
type MapView struct {
	ViewBox    string
	Width      float64
	Height     float64
	Background string // BackgroundHex or BackgroundImage
	Image      string // the uploaded image's file name, for the web layer to link
	HexPoints  string
	Cols       int
	Rows       int
	Cells      []Cell
	Coast      string     // path data along every sea/land hex side
	EdgePaths  []EdgePath // one path per edge type (river, ridge, ...)
	Crossings  []CrossingMark
	Roads      string // path data joining hex centers
	Grid       string // GridNone, GridHex, GridSquare
	GridPath   string // the grid overlay
	Scale      ScaleBar
	K          float64 // how much to scale counters and labels: one grid cell to one drawn hex
	Features   []FeatureMark
	Tethers    []Tether
	Stacks     []Stack        // one per visible unit, at its point
	Proposed   []ProposedMark // referee only: where a pending event would act
	OrderMarks []OrderMark    // spots players marked in their orders
}

// ScaleBar is drawn in a corner: so many board units are so far in the world.
type ScaleBar struct {
	X, Y, Length float64
	Label        string
}

// OrderMark is a spot a player marked in an order.
type OrderMark struct {
	X, Y   float64
	Label  string
	Status string
	Mine   bool
}

type Cell struct {
	X, Y  float64
	T     string // terrain id
	Hex   string
	Depth int // water only: distance to the nearest land, capped at 3
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

type ProposedMark struct {
	X, Y  float64
	Label string
}

type FeatureMark struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Glyph string  `json:"-"`
	FeatureView
}

// Stack is a counter on the board: one unit, at its point (nudged a little
// when several sit on the same spot).
type Tether struct{ X1, Y1, X2, Y2 float64 }
type Pip struct {
	X        float64
	Detached bool
}
type Stack struct {
	Members string
	Pips    []Pip
	Chit    bool
	X, Y    float64
	Top     UnitView
	Count   int
	Label   string // accessible description
}

// InspectorData is the JSON the client uses: the board's measures, orders,
// and (for the editor) the drawn map's terrain and hex sides.
type InspectorData struct {
	Plans   []*Plan               `json:"plans,omitempty"`
	Width   float64               `json:"width"`
	Height  float64               `json:"height"`
	Cell    float64               `json:"cell"`
	Grid    string                `json:"grid"`
	Hexes   bool                  `json:"hexes"` // the background is the drawn hex map
	Cols    int                   `json:"cols"`
	Rows    int                   `json:"rows"`
	Terrain map[string]Terrain    `json:"terrain"`
	Sides   map[string][]SideInfo `json:"sides"` // edges and roads, listed from each hex's point of view
	Orders  OrderData             `json:"orders"`
	// The board, for the pointer: what's where, and how far is how far.
	K        float64       `json:"k"`
	Distance Measure       `json:"distance"`
	CellName string        `json:"cellName"`
	Units    []UnitView    `json:"units"`
	Places   []FeatureMark `json:"places"`
	Referee  bool          `json:"referee,omitempty"`
}

type SideInfo struct {
	Dir       int    `json:"dir"`
	DirName   string `json:"dirName"`
	Edge      string `json:"edge,omitempty"`
	EdgeName  string `json:"edgeName,omitempty"`
	Crossing  string `json:"crossing,omitempty"`
	CrossName string `json:"crossingName,omitempty"`
	Blocks    bool   `json:"blocks,omitempty"` // an obstacle here
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

func buildMap(r *Room, mod *Module, me *Member, units []UnitView) (MapView, []Terrain, InspectorData) {
	ref := me.Role == RoleReferee
	m := r.Map
	rows := m.NumRows()
	w, h := r.Size()
	b := r.Board
	mv := MapView{
		ViewBox: fmt.Sprintf("0 0 %.1f %.1f", w, h), Width: w, Height: h, Cols: m.Cols, Rows: rows,
		Background: b.Background, Image: b.Image, Grid: b.Grid,
		HexPoints: fmt.Sprintf("0,%.1f %.2f,%.1f %.2f,%.1f 0,%.1f %.2f,%.1f %.2f,%.1f",
			-hexSize, hexW/2, -hexSize/2, hexW/2, hexSize/2, hexSize, -hexW/2, hexSize/2, -hexW/2, -hexSize/2),
	}
	data := InspectorData{Width: w, Height: h, Cell: b.Cell, Grid: b.Grid, Hexes: b.Background == BackgroundHex,
		Cols: m.Cols, Rows: rows, Terrain: map[string]Terrain{}, Sides: map[string][]SideInfo{}}
	var legend []Terrain
	if b.Background == BackgroundHex {
		legend = drawHexMap(r, mod, &mv, &data)
	}
	mv.K = math.Max(0.4, math.Min(40, b.Cell/hexW))
	data.K, data.Distance, data.CellName, data.Referee = mv.K, b.Distance, b.CellName(), ref
	mv.GridPath = gridPath(b, w, h)
	mv.Scale = scaleBar(b, w, h)

	for _, f := range r.Features {
		if f.Hidden && !ref {
			continue
		}
		fv := FeatureView{Name: f.Name, Kind: f.Kind, Where: r.GridRef(f.Pos()), Note: f.Note, Hidden: f.Hidden}
		if ref {
			fv.ID = f.ID
		}
		mv.Features = append(mv.Features, FeatureMark{X: f.X, Y: f.Y, Glyph: featureGlyphs[f.Kind], FeatureView: fv})
	}
	data.Places = mv.Features

	// Counters: the viewer's own on top, then friendly, then the rest. Ones
	// on the same spot fan out a little so each can be seen and clicked.
	// Build only from the filtered (and possibly frozen) view, never live units.
	var us []UnitView
	formations := map[string][]UnitView{}
	for _, u := range units {
		if u.Formation != "" && !u.Stale && !u.Detached && r.formation(u.Formation) != nil {
			formations[u.Formation] = append(formations[u.Formation], u)
		} else {
			us = append(us, u)
		}
	}
	for _, f := range r.Formations {
		ms := formations[f.ID]
		if len(ms) == 0 {
			continue
		}
		u := ms[0]
		u.ID, u.Name, u.Abbr, u.Card = FormationToken(f.ID), f.Name, f.Abbr, ms[0].ID
		u.ClassName, u.Strength, u.Status, u.AssignedName, u.Radio = "Formation", "", "", "", ""
		u.Mine = false
		for _, m := range ms {
			u.Members = append(u.Members, m.ID)
			u.Mine = u.Mine || m.Mine
		}
		us = append(us, u)
	}
	data.Units = slices.Clone(units)
	sort.SliceStable(us, func(i, j int) bool { return stackRank(us[i]) > stackRank(us[j]) })
	var placed []Pos
	nudge := 22 * mv.K
	for _, u := range us {
		// Spread counters that share a spot around it, in widening rings.
		at := Pos{u.X, u.Y}
		p := at
		taken := func(p Pos) bool {
			return slices.ContainsFunc(placed, func(q Pos) bool { return math.Hypot(q.X-p.X, q.Y-p.Y) < nudge*0.9 })
		}
		for i := 0; i < 24 && taken(p); i++ {
			ring := float64(i/6 + 1)
			a := float64(i%6)*math.Pi/3 + ring*0.5
			p = Pos{at.X + ring*nudge*math.Cos(a), at.Y + ring*nudge*math.Sin(a)}
		}
		placed = append(placed, p)
		label := u.Name
		if u.Where != "" {
			label += ", " + u.Where
		}
		if u.Stale {
			label += fmt.Sprintf(" (out of contact: where they last called in, turn %d)", u.SeenTurn)
		}
		st := Stack{X: p.X, Y: p.Y, Top: u, Count: 1, Label: label, Chit: u.Detached, Members: strings.Join(u.Members, " ")}
		if len(u.Members) > 0 {
			for _, m := range units {
				if m.Formation == u.Formation && !m.Stale {
					st.Pips = append(st.Pips, Pip{Detached: m.Detached})
				}
			}
			for i := range st.Pips {
				st.Pips[i].X = (float64(i) - float64(len(st.Pips)-1)/2) * 3
			}
			u.X, u.Y = p.X, p.Y
			data.Units = append(data.Units, u)
		}
		mv.Stacks = append(mv.Stacks, st)
	}
	for _, st := range mv.Stacks {
		if !st.Chit || st.Top.Stale {
			continue
		}
		for _, anchor := range mv.Stacks {
			if len(anchor.Pips) > 0 && anchor.Top.Formation == st.Top.Formation {
				mv.Tethers = append(mv.Tethers, Tether{st.X, st.Y, anchor.X, anchor.Y})
			}
		}
	}
	return mv, legend, data
}

// drawHexMap fills in the drawn hex map: terrain, coast, edges, crossings,
// and roads. It returns the terrain used, for the key.
func drawHexMap(r *Room, mod *Module, mv *MapView, data *InspectorData) []Terrain {
	m := r.Map
	rows := m.NumRows()
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
			for _, dir := range []int{0, 4, 5} {
				if nc, nr, ok := m.Neighbor(col, row, dir); ok && water(col, row) != water(nc, nr) {
					x1, y1, x2, y2 := sideEnds(col, row, dir)
					fmt.Fprintf(&coast, "M%.1f %.1fL%.1f %.1f", x1, y1, x2, y2)
				}
			}
		}
	}
	mv.Coast = coast.String()
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

	return legend
}

// gridPath draws the grid overlay: square lines, or hex outlines laid out
// like the drawn map's hexes (so with the drawn map's cell they line up).
func gridPath(b Board, w, h float64) string {
	var p strings.Builder
	c := b.Cell
	switch b.Grid {
	case GridSquare:
		for x := c; x < w; x += c {
			fmt.Fprintf(&p, "M%.1f 0V%.1f", x, h)
		}
		for y := c; y < h; y += c {
			fmt.Fprintf(&p, "M0 %.1fH%.1f", y, w)
		}
	case GridHex:
		size := c / math.Sqrt(3)
		n := 0
		for row := 0; 1.5*size*float64(row) < h+size && n < 12000; row++ {
			for col := 0; c*float64(col) < w+c && n < 12000; col++ {
				x := c*(float64(col)+0.5*float64(row&1)) + c/2
				y := 1.5*size*float64(row) + size
				fmt.Fprintf(&p, "M%.1f %.1fl%.2f %.2fv%.2fl%.2f %.2fl%.2f %.2fv%.2fz",
					x, y-size, c/2, size/2, size, -c/2, size/2, -c/2, -size/2, -size)
				n++
			}
		}
	}
	return p.String()
}

// scaleBar picks a round length near a fifth of the board's width.
func scaleBar(b Board, w, h float64) ScaleBar {
	if b.Distance.Amount == 0 || b.Cell <= 0 {
		return ScaleBar{}
	}
	target := w / 5 / b.Cell * b.Distance.Amount // in world units
	nice := 1.0
	for _, step := range []float64{1, 2, 5} {
		for mag := 1e-3; mag <= 1e6; mag *= 10 {
			if v := step * mag; math.Abs(v-target) < math.Abs(nice-target) {
				nice = v
			}
		}
	}
	length := nice / b.Distance.Amount * b.Cell
	return ScaleBar{X: b.Cell * 0.5, Y: h - b.Cell*0.6, Length: length, Label: Measure{Amount: nice, Unit: b.Distance.Unit}.String()}
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
