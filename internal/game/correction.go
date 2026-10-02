package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Corrections. Publishing keeps a snapshot of the game as it stood just
// before. Until the next turn locks, the referee can reopen the published
// turn to fix a mistake: the game goes back to the snapshot, still being
// resolved with its rolls and outcomes as they were, so the referee
// can reroll, rewrite, or adjust a unit and publish again. People, messages,
// invites, and the activity log aren't part of the game state and stay as
// they are. Orders already written for the new turn are set aside: they
// were written against a situation that's about to change.

// Undo is the snapshot taken when a turn was published.
type Undo struct {
	Turn  int             `json:"turn"` // the turn that was published
	At    time.Time       `json:"at"`
	State json.RawMessage `json:"state"`
}

// Correction records a reopened turn.
type Correction struct {
	Turn   int       `json:"turn"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}

// gameState is the part of a room a correction rolls back: pointers into
// the room, so the same value both saves and restores it.
type gameState struct {
	Turn        *int           `json:"turn"`
	Phase       *Phase         `json:"phase"`
	Deadline    *time.Time     `json:"deadline"`
	Deferred    *bool          `json:"deferred"`
	LockedAt    *time.Time     `json:"lockedAt"`
	Threat      *int           `json:"threat"`
	Units       *[]*Unit       `json:"units"`
	Features    *[]Feature     `json:"features"`
	Events      *[]*GameEvent  `json:"events"`
	Orders      *[]*Order      `json:"orders"`
	Desk        **Desk         `json:"desk"`
	LockedUnits *[]*Unit       `json:"lockedUnits"`
	Reports     *[]*TurnReport `json:"reports"`
	Seq         *int           `json:"seq"`
}

func (r *Room) gameState() gameState {
	return gameState{&r.Turn, &r.Phase, &r.Deadline, &r.Deferred, &r.LockedAt, &r.Threat, &r.Units, &r.Features,
		&r.Events, &r.Orders, &r.Desk, &r.LockedUnits, &r.Reports, &r.Seq}
}

// snapshot saves the game state for a correction of this turn.
func (r *Room) snapshot(now time.Time) {
	b, err := json.Marshal(r.gameState())
	if err != nil {
		r.Undo = nil
		return
	}
	r.Undo = &Undo{Turn: r.Turn, At: now.UTC(), State: b}
}

// CanCorrect reports whether the last published turn can still be reopened.
func (r *Room) CanCorrect() bool {
	return r.Undo != nil && r.Phase == PhasePlanning && r.Turn == r.Undo.Turn+1 && !r.Closed
}

// NewTurnOrders counts the orders that reopening would set aside.
func (r *Room) NewTurnOrders() int {
	n := 0
	for _, o := range r.Orders {
		if o.Turn == r.Turn && o.Status != OrderWithdrawn {
			n++
		}
	}
	return n
}

// ReopenTurn rolls the game back to just before the last publish.
func (r *Room) ReopenTurn(actor *Member, reason string, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if !r.CanCorrect() {
		return bad("only the last published turn can be reopened, and only until the next turn locks")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 300 {
		return bad("say what needs correcting (up to 300 characters); players see it")
	}
	setAside := r.NewTurnOrders()
	undo := r.Undo
	// Decoding into existing slices and maps would merge with what's there;
	// start them empty so the snapshot replaces them outright.
	r.Units, r.Features, r.Events, r.Orders, r.Desk, r.LockedUnits, r.Reports = nil, nil, nil, nil, nil, nil, nil
	st := r.gameState()
	if err := json.Unmarshal(undo.State, &st); err != nil {
		return fmt.Errorf("restoring turn %d: %w", undo.Turn, err)
	}
	r.Undo = nil
	r.LockedAt = now.UTC() // the referee gets a full window before any takeover
	r.Corrections = append(r.Corrections, Correction{Turn: r.Turn, At: now.UTC(), Reason: reason})
	line := fmt.Sprintf("The referee reopened turn %d to correct it: %s", r.Turn, reason)
	if setAside > 0 {
		line += fmt.Sprintf(" %s for turn %d %s set aside; you'll write them again once the corrected turn is published.",
			plural(setAside, "order", "orders"), r.Turn+1, map[bool]string{true: "was", false: "were"}[setAside == 1])
	}
	r.logKind(VisAll, "sitrep", "%s", line)
	r.notifyAll(actor.ID, fmt.Sprintf("Turn %d reopened", r.Turn), line, "reports")
	return nil
}

// pendingCorrection is the reason this turn was reopened, if it was.
func (r *Room) pendingCorrection() string {
	for i := len(r.Corrections) - 1; i >= 0; i-- {
		if c := r.Corrections[i]; c.Turn == r.Turn {
			return c.Reason
		}
	}
	return ""
}
