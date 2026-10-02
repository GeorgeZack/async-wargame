package game

import "fmt"

// The referee's moves. When the referee moves a counter during play (a
// drag on the map, a group move, the editor), the move is noted for the
// turn: where it started and where it ended up. It's information for
// judging, shown beside the unit's order on the desk; players never see
// these notes, only the counter where it now stands.

// Move is one unit's movement by the referee during a turn.
type Move struct {
	Turn int    `json:"turn"`
	Unit string `json:"unit"`
	From Pos    `json:"from"`
	To   Pos    `json:"to"`
}

// noteMove records that the referee moved u from a point. Several moves in
// a turn add up to one; moving it back where it started forgets it.
func (r *Room) noteMove(u *Unit, from Pos) {
	if r.Phase == PhaseSetup || u.Pos() == from {
		return
	}
	for i, m := range r.Moves {
		if m.Turn == r.Turn && m.Unit == u.ID {
			m.To = u.Pos()
			if r.Cells(m.From, m.To) < 0.05 {
				r.Moves = append(r.Moves[:i], r.Moves[i+1:]...)
			}
			return
		}
	}
	r.Moves = append(r.Moves, &Move{Turn: r.Turn, Unit: u.ID, From: from, To: u.Pos()})
	// Keep a few turns.
	kept := r.Moves[:0]
	for _, m := range r.Moves {
		if m.Turn >= r.Turn-3 {
			kept = append(kept, m)
		}
	}
	r.Moves = kept
}

// MoveView is a move as the referee reads it.
type MoveView struct {
	Unit     string // the unit's name
	UnitID   string
	From, To string // where, in words
	Distance string
	Text     string // "You moved it from 0509, at Red Beach to 1008, at Canal bridge (about 93 m)."
}

func (r *Room) moveView(m *Move) MoveView {
	mv := MoveView{UnitID: m.Unit, From: r.Where(m.From, true), To: r.Where(m.To, true), Distance: r.DistanceText(m.From, m.To)}
	if u := r.Unit(m.Unit); u != nil {
		mv.Unit = u.Name
	}
	from, to := mv.From, mv.To
	if from == "" {
		from = "where it was"
	}
	if to == "" {
		to = "a new spot"
	}
	mv.Text = fmt.Sprintf("You moved it from %s to %s (%s).", from, to, mv.Distance)
	return mv
}

// movesThisTurn are the referee's moves this turn, oldest first.
func (r *Room) movesThisTurn() []*Move {
	var out []*Move
	for _, m := range r.Moves {
		if m.Turn == r.Turn {
			out = append(out, m)
		}
	}
	return out
}

// MoveOf is a unit's move this turn, if the referee moved it.
func (r *Room) MoveOf(unitID string) *MoveView {
	for _, m := range r.movesThisTurn() {
		if m.Unit == unitID {
			mv := r.moveView(m)
			return &mv
		}
	}
	return nil
}
