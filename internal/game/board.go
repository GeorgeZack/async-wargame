package game

import (
	"fmt"
	"math"
	"strings"
)

// The board. Markers sit anywhere on it: there's no grid to snap to. The
// background is either the drawn hex map (terrain, rivers, roads) or an
// image the referee uploads: a sketch, a scanned map, a city plan. A grid
// can be laid over either (hexes or squares, or none) as a shared way of
// saying where things are and how far apart, and the scale says what one
// cell of it is in the world: "1 hex = 50 m". The turn says how much time
// one turn covers.

// Pos is a point on the board, in board units: SVG units for the drawn
// map, pixels for an image.
type Pos struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Measure is an amount and a unit: 50 m, 10 minutes.
type Measure struct {
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

func (m Measure) String() string {
	if m.Amount == 0 || m.Unit == "" {
		return ""
	}
	a := strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", m.Amount), "0"), ".")
	u := m.Unit
	if m.Amount == 1 {
		u = strings.TrimSuffix(u, "s") // "1 minute", "1 hour"
	}
	return a + " " + u
}

// Backgrounds and grids.
const (
	BackgroundHex   = "hex"
	BackgroundImage = "image"
	GridNone        = "none"
	GridHex         = "hex"
	GridSquare      = "square"
)

// Board is how the map looks and what it measures.
type Board struct {
	Background string  `json:"background"`      // BackgroundHex or BackgroundImage
	Image      string  `json:"image,omitempty"` // the uploaded file's name
	ImageW     int     `json:"imageW,omitempty"`
	ImageH     int     `json:"imageH,omitempty"`
	Grid       string  `json:"grid"` // GridNone, GridHex, GridSquare
	Cell       float64 `json:"cell"` // size of one grid cell, in board units
	Distance   Measure `json:"distance"`
	Turn       Measure `json:"turn"`
}

// Units the referee can pick.
var (
	DistanceUnits = []string{"ft", "yd", "m", "km", "mi"}
	TimeUnits     = []string{"seconds", "minutes", "hours", "days", "weeks"}
)

// withDefaults fills in anything a scenario or an older room left out.
func (b Board) withDefaults() Board {
	if b.Background == "" {
		b.Background = BackgroundHex
	}
	if b.Grid == "" {
		b.Grid = GridHex
	}
	if b.Cell <= 0 {
		b.Cell = hexW // one drawn hex
		if b.Background == BackgroundImage && b.ImageW > 0 {
			b.Cell = float64(b.ImageW) / 20
		}
	}
	return b
}

// CellName is what one grid cell is called in scale text.
func (b Board) CellName() string {
	switch b.Grid {
	case GridSquare:
		return "square"
	case GridHex:
		return "hex"
	}
	return "grid cell"
}

// ScaleText is the scale and the turn in a line: "1 hex = 50 m · 1 turn = 10 minutes".
func (b Board) ScaleText() string {
	var parts []string
	if d := b.Distance.String(); d != "" {
		parts = append(parts, fmt.Sprintf("1 %s = %s", b.CellName(), d))
	}
	if t := b.Turn.String(); t != "" {
		parts = append(parts, "1 turn = "+t)
	}
	return strings.Join(parts, " · ")
}

// Size is the board's extent in board units.
func (r *Room) Size() (w, h float64) {
	if r.Board.Background == BackgroundImage && r.Board.ImageW > 0 {
		return float64(r.Board.ImageW), float64(r.Board.ImageH)
	}
	return hexW * (float64(r.Map.Cols) + 0.5), 1.5*hexSize*float64(r.Map.NumRows()-1) + 2*hexSize
}

// clampPos keeps a point on the board.
func (r *Room) clampPos(p Pos) Pos {
	w, h := r.Size()
	return Pos{X: math.Min(w, math.Max(0, p.X)), Y: math.Min(h, math.Max(0, p.Y))}
}

// Cells is how many grid cells apart two points are, as the crow flies.
func (r *Room) Cells(a, b Pos) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y) / r.Board.Cell
}

// DistanceText says how far apart two points are in the world: "about 150 m".
func (r *Room) DistanceText(a, b Pos) string {
	n := r.Cells(a, b)
	d := r.Board.Distance
	if d.Amount == 0 {
		return fmt.Sprintf("about %.1f %ss", n, r.Board.CellName())
	}
	return "about " + Measure{Amount: math.Round(n*d.Amount*10) / 10, Unit: d.Unit}.String()
}

// GridRef names the grid cell a point is in: "0905" on a hex grid, "C4"
// on squares, "" with no grid.
func (r *Room) GridRef(p Pos) string {
	switch r.Board.Grid {
	case GridSquare:
		col, row := int(p.X/r.Board.Cell), int(p.Y/r.Board.Cell)
		return colLetters(col) + fmt.Sprint(row+1)
	case GridHex:
		col, row := hexAtScaled(p, r.Board.Cell)
		return HexID(col, row)
	}
	return ""
}

func colLetters(n int) string {
	s := ""
	for n >= 0 {
		s = string(rune('A'+n%26)) + s
		n = n/26 - 1
	}
	return s
}

// hexAtScaled finds the pointy-top, odd-row-offset hex containing p, for
// hexes cell wide (the drawn map's layout when cell is one drawn hex).
func hexAtScaled(p Pos, cell float64) (int, int) {
	size := cell / math.Sqrt(3)
	best, bd := [2]int{}, math.Inf(1)
	r0 := int(math.Round((p.Y - size) / (1.5 * size)))
	for row := r0 - 1; row <= r0+1; row++ {
		if row < 0 {
			continue
		}
		c0 := int(math.Round((p.X-cell/2)/cell - 0.5*float64(row&1)))
		for col := c0 - 1; col <= c0+1; col++ {
			if col < 0 {
				continue
			}
			x := cell*(float64(col)+0.5*float64(row&1)) + cell/2
			y := 1.5*size*float64(row) + size
			if d := (x-p.X)*(x-p.X) + (y-p.Y)*(y-p.Y); d < bd {
				best, bd = [2]int{col, row}, d
			}
		}
	}
	return best[0], best[1]
}

// TerrainAt is the drawn map's terrain under a point, or nil on an image.
func (r *Room) TerrainAt(mod *Module, p Pos) *Terrain {
	if r.Board.Background != BackgroundHex {
		return nil
	}
	col, row := hexAtScaled(p, hexW)
	if !r.Map.InBounds(col, row) {
		return nil
	}
	return mod.TerrainAt(r.Map.TerrainCode(col, row))
}

// Where describes a point in words: the nearest place, and the grid
// reference. "0905, at St. Ines Hospital"; "near the canal bridge"; "C4".
func (r *Room) Where(p Pos, ref bool) string {
	var place string
	best := math.Inf(1)
	for _, f := range r.Features {
		if f.Hidden && !ref {
			continue
		}
		if d := r.Cells(p, f.Pos()); d < best && d <= 1.5 {
			best = d
			switch {
			case d <= 0.6:
				place = "at " + f.Name
			default:
				place = "near " + f.Name
			}
		}
	}
	g := r.GridRef(p)
	switch {
	case g != "" && place != "":
		return g + ", " + place
	case g != "":
		return g
	}
	return place
}

// Pos is a unit's position.
func (u *Unit) Pos() Pos { return Pos{u.X, u.Y} }

// Pos is a place's position.
func (f Feature) Pos() Pos { return Pos{f.X, f.Y} }

// center is the point at the middle of a drawn hex.
func center(col, row int) Pos {
	x, y := HexCenter(col, row)
	return Pos{x, y}
}

// BoardEdit is the referee changing the board.
type BoardEdit struct {
	Background string
	Grid       string
	Cell       float64
	Distance   Measure
	Turn       Measure
}

// SetBoard changes the background, grid, and scale. Markers keep their
// points; switching backgrounds may leave some off the new board, and
// they're pulled back onto its edge.
func (r *Room) SetBoard(actor *Member, e BoardEdit) error {
	if r.MissionComplete() {
		return bad("reopen the final turn before changing the completed mission")
	}
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	b := r.Board
	switch e.Background {
	case BackgroundHex:
	case BackgroundImage:
		if b.Image == "" {
			return bad("upload a map image first")
		}
	default:
		return bad("the background is the drawn hex map or an uploaded image")
	}
	if e.Grid != GridNone && e.Grid != GridHex && e.Grid != GridSquare {
		return bad("the grid is none, hex, or square")
	}
	if e.Distance.Amount < 0 || e.Distance.Amount > 1e6 || e.Turn.Amount < 0 || e.Turn.Amount > 1e6 {
		return bad("the scale must be a sensible number")
	}
	if e.Distance.Amount > 0 && !slicesContains(DistanceUnits, e.Distance.Unit) {
		return bad("pick a distance unit")
	}
	if e.Turn.Amount > 0 && !slicesContains(TimeUnits, e.Turn.Unit) {
		return bad("pick a time unit")
	}
	b.Background, b.Grid, b.Distance, b.Turn = e.Background, e.Grid, e.Distance, e.Turn
	w, h := r.sizeFor(b)
	switch {
	case e.Background == BackgroundHex && e.Cell <= 0:
		b.Cell = hexW
	case e.Cell <= 0:
		b.Cell = w / 20
	default:
		b.Cell = e.Cell
	}
	if b.Cell < math.Min(w, h)/200 || b.Cell > math.Max(w, h) {
		return bad("that grid size doesn't fit the map")
	}
	r.Board = b
	for _, u := range r.Units {
		p := r.clampPos(u.Pos())
		u.X, u.Y = p.X, p.Y
	}
	for i := range r.Features {
		p := r.clampPos(r.Features[i].Pos())
		r.Features[i].X, r.Features[i].Y = p.X, p.Y
	}
	r.logf(VisAll, "The referee changed the map: %s.", r.Board.ScaleText())
	return nil
}

// SetBoardImage records an uploaded map image and switches to it. Markers
// on the old background are scaled to the same relative places.
func (r *Room) SetBoardImage(actor *Member, name string, w, h int) error {
	if r.MissionComplete() {
		return bad("reopen the final turn before changing the completed mission")
	}
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if w < 100 || h < 100 || w > 20000 || h > 20000 {
		return bad("the image should be between 100 and 20,000 pixels on each side")
	}
	ow, oh := r.Size()
	cellFrac := r.Board.Cell / ow
	r.Board.Image, r.Board.ImageW, r.Board.ImageH, r.Board.Background = name, w, h, BackgroundImage
	nw, nh := float64(w), float64(h)
	sx, sy := nw/ow, nh/oh
	for _, u := range r.Units {
		u.X, u.Y = u.X*sx, u.Y*sy
		if u.Reported != nil {
			u.Reported = &Pos{u.Reported.X * sx, u.Reported.Y * sy}
		}
	}
	for i := range r.Features {
		r.Features[i].X, r.Features[i].Y = r.Features[i].X*sx, r.Features[i].Y*sy
	}
	r.Board.Cell = cellFrac * nw
	r.logf(VisAll, "The referee put up a new map.")
	return nil
}

func (r *Room) sizeFor(b Board) (float64, float64) {
	old := r.Board
	r.Board = b
	w, h := r.Size()
	r.Board = old
	return w, h
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
