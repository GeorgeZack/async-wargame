package game

import (
	"fmt"
	"math"
)

// Movement. Each kind of unit has a careful pace and a run (km/h, in the
// module). With the board's scale and the length of a turn, that says how
// far a unit can realistically get in one turn, in the world and in grid
// cells, so an order to "go ten hexes" can be read against what's possible.

// metersPer is how many meters one board distance unit is.
var metersPer = map[string]float64{"ft": 0.3048, "yd": 0.9144, "m": 1, "km": 1000, "mi": 1609.344}

// minutesPer is how many minutes one turn unit is.
var minutesPer = map[string]float64{"seconds": 1.0 / 60, "minutes": 1, "hours": 60, "days": 1440, "weeks": 10080}

// Reach is how far a unit gets in one turn.
type Reach struct {
	Careful, Run           float64 // meters per turn
	CarefulCells, RunCells float64
}

// ReachOf works out a unit's reach per turn; ok is false when the board
// has no scale or the unit's kind no speed.
func (r *Room) ReachOf(mod *Module, u *Unit) (Reach, bool) {
	cl := mod.Class(u.Class)
	b := r.Board
	cellM := b.Distance.Amount * metersPer[b.Distance.Unit]
	turnMin := b.Turn.Amount * minutesPer[b.Turn.Unit]
	if cl == nil || cl.Speed <= 0 || cellM <= 0 || turnMin <= 0 {
		return Reach{}, false
	}
	run := cl.Run
	if run <= 0 {
		run = cl.Speed * 3
	}
	perMin := func(kmh float64) float64 { return kmh * 1000 / 60 }
	re := Reach{Careful: perMin(cl.Speed) * turnMin, Run: perMin(run) * turnMin}
	re.CarefulCells, re.RunCells = re.Careful/cellM, re.Run/cellM
	return re, true
}

// meters prints a length in the board's own distance unit.
func (r *Room) lengthText(m float64) string {
	u := r.Board.Distance.Unit
	per := metersPer[u]
	if per == 0 {
		u, per = "m", 1
	}
	v := m / per
	switch {
	case v >= 100:
		v = math.Round(v/10) * 10
	case v >= 10:
		v = math.Round(v)
	default:
		v = math.Round(v*10) / 10
	}
	return "about " + Measure{Amount: v, Unit: u}.String()
}

func cellsText(n float64, name string) string {
	n = math.Round(n*10) / 10
	if n == 1 {
		return "1 " + name
	}
	if name == "hex" {
		name = "hexe"
	}
	return fmt.Sprintf("%g %ss", n, name)
}

// ReachText says a unit's reach per turn: "careful: about 100 m (6.7 hexes);
// at a run: about 400 m (27 hexes)".
func (r *Room) ReachText(mod *Module, u *Unit) string {
	re, ok := r.ReachOf(mod, u)
	if !ok {
		return ""
	}
	cell := r.Board.CellName()
	return fmt.Sprintf("careful pace %s (%s) a turn; at a run %s (%s)",
		r.lengthText(re.Careful), cellsText(re.CarefulCells, cell), r.lengthText(re.Run), cellsText(re.RunCells, cell))
}

// IntendedMove reads an order's pin as where the unit means to get to, and
// says how far that is against what the unit can do in a turn.
func (r *Room) IntendedMove(mod *Module, u *Unit, o *Order) string {
	if o == nil || o.Mark == nil {
		return ""
	}
	to := *o.Mark
	cells := r.Cells(u.Pos(), to)
	text := fmt.Sprintf("wants to reach %s: %s (%s) away", r.Where(to, true), r.lengthText(cells*r.Board.Distance.Amount*metersPer[r.Board.Distance.Unit]), cellsText(cells, r.Board.CellName()))
	if r.Board.Distance.Amount == 0 {
		text = fmt.Sprintf("wants to reach %s: %s away", r.Where(to, true), cellsText(cells, r.Board.CellName()))
	}
	re, ok := r.ReachOf(mod, u)
	if !ok || cells < 0.5 {
		return text
	}
	switch {
	case cells <= re.CarefulCells:
		text += "; within a turn at a careful pace"
	case cells <= re.RunCells:
		text += fmt.Sprintf("; too far at a careful pace (about %.1f turns), within reach at a run", cells/re.CarefulCells)
	default:
		text += fmt.Sprintf("; too far for one turn even at a run (about %.1f turns careful, %.1f running), so it gets part of the way", cells/re.CarefulCells, cells/re.RunCells)
	}
	return text
}
