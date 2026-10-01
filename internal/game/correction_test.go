package game

import (
	"strings"
	"testing"
)

func TestReopenAndCorrectATurn(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	var dest [2]int
	for h, st := range r.Reach(mod, u) {
		if st.Cost > 0 && r.fits(mod, u, h, r.ProjectedLoad(mod, u.ID, nil)) {
			dest = h
			break
		}
	}
	start := [2]int{u.Col, u.Row}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(dest[0], dest[1]), Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	if r.CanCorrect() {
		t.Error("correctable before anything was published")
	}
	publishTurn(t, r, mod, ref, now)
	// (A contact on the way may have stopped or hurt the squad; what matters
	// is that reopening puts it back exactly where it started.)
	if len(r.Reports) != 1 || !r.CanCorrect() {
		t.Fatalf("after publish: %d reports, correctable %v", len(r.Reports), r.CanCorrect())
	}

	// Turn 2: an order, and a message that must survive the correction.
	r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "cautious", Fallback: "hold", Commit: true}, now)
	r.Post(hicks, "all", "Dug in at the crossroads.", now)

	if err := r.ReopenTurn(hicks, "no", now); err != ErrForbidden {
		t.Errorf("a player reopened the turn: %v", err)
	}
	if err := r.ReopenTurn(ref, "  ", now); err == nil {
		t.Error("reopened without a reason")
	}
	if n := r.NewTurnOrders(); n != 1 {
		t.Errorf("orders to set aside: %d", n)
	}
	if err := r.ReopenTurn(ref, "the squad should have stopped at the bridge", now); err != nil {
		t.Fatal(err)
	}
	u = r.Unit("m1")
	if r.Turn != 1 || r.Phase != PhaseResolving || [2]int{u.Col, u.Row} != start || len(r.Reports) != 0 || r.Resolution == nil {
		t.Fatalf("after reopening: turn %d %s, m1 at %v, %d reports", r.Turn, r.Phase, [2]int{u.Col, u.Row}, len(r.Reports))
	}
	if len(r.Messages) != 1 {
		t.Error("the correction lost a message")
	}
	if !strings.Contains(r.Log[len(r.Log)-1].Text, "1 order for turn 2 was set aside") {
		t.Errorf("log: %q", r.Log[len(r.Log)-1].Text)
	}
	if r.CanCorrect() {
		t.Error("still correctable while being resolved again")
	}

	// Publish again: the report says it was corrected; turn 2 starts clean.
	publishTurn(t, r, mod, ref, now)
	if len(r.Reports) != 1 || r.Reports[0].Corrected != "the squad should have stopped at the bridge" {
		t.Fatalf("reports %+v", r.Reports)
	}
	if o := r.CurrentOrder("m1"); o != nil && o.Task == "dig_in" {
		t.Error("the set-aside order came back")
	}
	// Once turn 2 locks, turn 1 is final.
	r.Tick(mod, r.Deadline)
	if r.CanCorrect() || r.ReopenTurn(ref, "too late", now) == nil {
		t.Error("reopened turn 1 after turn 2 locked")
	}
}
