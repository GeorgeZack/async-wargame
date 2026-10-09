package game

import "math"

func (r *Room) enemyAwareness(mod *Module) Awareness {
	if r.Awareness != nil {
		return *r.Awareness
	}
	if mod.Awareness != nil {
		return *mod.Awareness
	}
	return Awareness{RangeCells: 3}
}

// Enemy knowledge is independent of what players know. A revealed unit may be
// noticed in cover, but its present position is not known through a wall or
// beyond observation range. Same-side units can coordinate in cover.
func (r *Room) enemyNotices(mod *Module, observer, target *Unit) bool {
	if target == observer || target.Destroyed() {
		return false
	}
	a := r.enemyAwareness(mod)
	distance := r.Cells(observer.Pos(), target.Pos())
	if distance > a.RangeCells {
		return false
	}
	if observer.Side == target.Side {
		return true
	}
	if !r.clearEnemySight(mod, observer.Pos(), target.Pos()) {
		return false
	}
	if t := r.TerrainAt(mod, target.Pos()); t != nil && t.Conceals && !target.RevealedToEnemy && distance > a.ConcealedCells {
		return false
	}
	return true
}

// Drawn maps have explicit terrain and edges. Image boards have no such
// geometry, so only range and recorded revelation apply there. Sampling at a
// small fraction of a terrain hex also handles off-centre counter positions.
func (r *Room) clearEnemySight(mod *Module, a, b Pos) bool {
	if r.Board.Background != BackgroundHex || r.Map == nil {
		return true
	}
	startCol, startRow := hexAtScaled(a, hexW)
	endCol, endRow := hexAtScaled(b, hexW)
	steps := max(1, int(math.Ceil(math.Hypot(b.X-a.X, b.Y-a.Y)/(hexW/8))))
	lastCol, lastRow := startCol, startRow
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		col, row := hexAtScaled(Pos{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f}, hexW)
		if !r.Map.InBounds(col, row) {
			return false
		}
		if col == lastCol && row == lastRow {
			continue
		}
		if side, ok := r.Map.SideBetween(lastCol, lastRow, col, row); ok {
			if edge := r.Map.EdgeAt(side); edge != nil {
				if typ := mod.EdgeType(edge.Type); typ != nil && typ.BlocksSight {
					return false
				}
			}
		}
		lastCol, lastRow = col, row
		if col == endCol && row == endRow || col == startCol && row == startRow {
			continue
		}
		if terrain := mod.TerrainAt(r.Map.TerrainCode(col, row)); terrain != nil && terrain.BlocksSight {
			return false
		}
	}
	return true
}
