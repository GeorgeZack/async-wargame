package game

import (
	"slices"
	"strings"
	"testing"
)

func TestStandingOrdersCarryOverAsDrafts(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	// Head for a hex beyond one turn's reach, then dig in.
	leg := [2]int{u.Col, u.Row}
	reach := r.Reach(mod, u)
	for h, st := range reach {
		if st.Cost > 0 && HexDistance(u.Col, u.Row, h[0], h[1]) > HexDistance(u.Col, u.Row, leg[0], leg[1]) {
			leg = h
		}
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(leg[0], leg[1]), Risk: "balanced", Fallback: "hold",
		Then: "move", Commit: true}, now); err == nil {
		t.Error("accepted a move as the follow-on task")
	}
	o, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(leg[0], leg[1]), Risk: "balanced", Fallback: "hold",
		Then: "dig_in", Standing: true, Commit: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.orderSummary(mod, o), "then dig in") {
		t.Errorf("summary %q", r.orderSummary(mod, o))
	}
	// A standing hold for the Weapons Squad's holder too.
	r.SaveOrder(r.Member("v"), mod, OrderInput{Unit: "m2", Task: "hold", Risk: "balanced", Fallback: "hold", Standing: true, Commit: true}, now)

	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	if r.Turn != 2 {
		t.Fatalf("turn %d", r.Turn)
	}
	next := r.CurrentOrder("m1")
	if [2]int{u.Col, u.Row} == leg {
		if next == nil || next.Task != "dig_in" || next.Status != OrderDraft || !next.Carried {
			t.Fatalf("arrived squad should dig in as a draft: %+v", next)
		}
	} else if next == nil || next.Status != OrderDraft {
		t.Fatalf("unfinished move should continue as a draft: %+v", next)
	}
	hold := r.CurrentOrder("m2")
	if hold == nil || hold.Task != "hold" || hold.Status != OrderDraft || !hold.Standing {
		t.Fatalf("standing hold: %+v", hold)
	}
	// Someone else can't confirm it; the holder can, all at once.
	if err := r.ConfirmOrder(hicks, mod, hold.ID, now); err == nil {
		t.Error("Hicks confirmed Vasquez's order")
	}
	if err := r.ConfirmAllStanding(hicks, mod, now); err != nil {
		t.Fatal(err)
	}
	if next.Status != OrderCommitted || hold.Status != OrderDraft {
		t.Fatalf("after confirm-all: m1 %s, m2 %s", next.Status, hold.Status)
	}
	// Unconfirmed, the hold still runs at the lock, defensively only.
	r.Tick(mod, r.Deadline)
	if hold.Status != OrderLocked || !hold.Unconfirmed {
		t.Fatalf("unconfirmed standing order: %s, unconfirmed %v", hold.Status, hold.Unconfirmed)
	}
	if !strings.Contains(r.orderSummary(mod, hold), "defensively") {
		t.Errorf("summary %q", r.orderSummary(mod, hold))
	}
	if !slices.ContainsFunc(r.Resolution.Notes, func(n string) bool { return strings.Contains(n, "Unconfirmed standing orders") }) {
		t.Errorf("notes %v", r.Resolution.Notes)
	}
	if f := r.sideState(mod, []string{"m2"}, nil, map[string][2]int{"m2": {r.Unit("m2").Col, r.Unit("m2").Row}}, map[string][2]int{"m2": {r.Unit("m2").Col, r.Unit("m2").Row}}, true); !f.Fighters[0].Missed {
		t.Error("an unconfirmed standing order fights at full strength")
	}
	// It keeps coming back until someone deals with it.
	publishTurn(t, r, mod, ref, now)
	if o := r.CurrentOrder("m2"); o == nil || o.Task != "hold" || !o.Carried {
		t.Errorf("the standing hold didn't carry over again: %+v", o)
	}
}
