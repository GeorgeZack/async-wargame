package game

import (
	"slices"
	"strings"
	"testing"
)

func TestStackingLimitsOrders(t *testing.T) {
	r, mod, _, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	r.Map.SetTerrain(7, 7, 'm') // marsh holds 3 points
	r.Unit("m5").AssignedTo = "h"
	dest := HexID(7, 7)
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m5", Task: "move", Dest: dest, Risk: "balanced", Fallback: "hold"}, now); err != nil {
		t.Fatalf("first squad into the marsh: %v", err)
	}
	_, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: dest, Risk: "balanced", Fallback: "hold"}, now)
	if err == nil || !strings.Contains(err.Error(), "is full") {
		t.Errorf("second squad into a full marsh: %v", err)
	}
}

func TestOrderTheWholeStack(t *testing.T) {
	r, mod, _, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	r.Unit("m3").Col, r.Unit("m3").Row, r.Unit("m3").AssignedTo = 5, 7, "h" // HMG joins the 1st Rifle Squad
	orders, err := r.SaveOrders(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(6, 6), Risk: "balanced", Fallback: "hold", WithStack: true}, now)
	if err != nil || len(orders) != 2 {
		t.Fatalf("stack order: %v, %d orders", err, len(orders))
	}
	if r.plannedHex(r.Unit("m3")) != [2]int{6, 6} {
		t.Error("the HMG didn't get the order")
	}
}

func TestResolutionStopsShortOfAFullHex(t *testing.T) {
	r, mod, _, _, hicks := orderRoom(t)
	r.Units = slices.DeleteFunc(r.Units, func(u *Unit) bool { return u.Side == "infected" })
	now := central(t, "2026-09-30 13:00")
	r.Map.SetTerrain(7, 7, 'm') // holds 3
	r.Unit("m5").AssignedTo = "h"
	// The sappers plan to leave the marsh hex... then never commit, so they stay.
	r.Unit("m5").Col, r.Unit("m5").Row = 7, 7
	r.SaveOrder(hicks, mod, OrderInput{Unit: "m5", Task: "move", Dest: HexID(8, 7), Risk: "balanced", Fallback: "hold"}, now)
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(7, 7), Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatalf("planned into the space the sappers were leaving: %v", err)
	}
	r.Tick(mod, r.Deadline)
	if !slices.ContainsFunc(r.Resolution.Notes, func(n string) bool { return strings.Contains(n, "was full") }) {
		t.Errorf("the squad didn't stop short of the still-occupied marsh: %v", r.Resolution.Notes)
	}
	publishTurn(t, r, mod, r.Referee(), now)
	if used, limit := r.Load(mod)[[2]int{7, 7}], r.StackLimit(mod, 7, 7); used > limit {
		t.Errorf("marsh overstacked: %d of %d", used, limit)
	}
}

func TestEditorRespectsStacking(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	r.Map.SetTerrain(10, 1, 'm')
	ops := []EditOp{{Op: "unit-add", Class: "rifle_squad", Hex: HexID(10, 1)}, {Op: "unit-add", Class: "rifle_squad", Hex: HexID(10, 1)}}
	if err := r.ApplyEdits(ref, mod, ops); err == nil || !strings.Contains(err.Error(), "full") {
		t.Errorf("two squads in one marsh hex: %v", err)
	}
	// The infected don't count against stacking.
	ops = []EditOp{{Op: "unit-add", Class: "horde", Hex: HexID(10, 1), Strength: 50}, {Op: "unit-add", Class: "horde", Hex: HexID(10, 1), Strength: 50}}
	if err := r.ApplyEdits(ref, mod, ops); err != nil {
		t.Errorf("hordes limited by stacking: %v", err)
	}
}

func TestScenarioStartsWithinStackingLimits(t *testing.T) {
	r, mod, _, _ := testRoom(t)
	for h, used := range r.Load(mod) {
		if limit := r.StackLimit(mod, h[0], h[1]); used > limit {
			t.Errorf("%s starts with %d of %d points", HexID(h[0], h[1]), used, limit)
		}
	}
}
