package game

import (
	"slices"
	"strings"
	"testing"
)

func orderRoom(t *testing.T) (*Room, *Module, *Member, *Member, *Member) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	cmd := &Member{ID: "c", Name: "Apone", Role: RoleCommander, Status: StatusApproved}
	hicks := &Member{ID: "h", Name: "Hicks", Role: RolePlayer, Status: StatusApproved, ReportsTo: "c"}
	vasq := &Member{ID: "v", Name: "Vasquez", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, cmd, hicks, vasq)
	r.Unit("m1").AssignedTo = "h"
	r.Unit("m2").AssignedTo = "v"
	return r, mod, ref, cmd, hicks
}

func TestOrderPermissions(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	cases := []struct {
		who  *Member
		unit string
		ok   bool
	}{
		{hicks, "m1", true}, {hicks, "m2", false}, {cmd, "m1", true}, {cmd, "m2", false},
		{vasq, "m1", false}, {ref, "m3", true}, {ref, "z1", false},
	}
	for _, c := range cases {
		if got := r.CanOrder(mod, c.who, r.Unit(c.unit)); got != c.ok {
			t.Errorf("%s ordering %s: %v, want %v", c.who.Name, c.unit, got, c.ok)
		}
	}
}

func TestReachRespectsMovementAndRivers(t *testing.T) {
	r, mod, _, _, _ := orderRoom(t)
	u := r.Unit("m2") // 2nd Rifle Squad, south of the river at 0611
	reach := r.Reach(mod, u)
	mv := mod.Class(u.Class).Movement
	for h, st := range reach {
		if st.Cost > mv {
			t.Errorf("%s costs %d, over movement %d", HexID(h[0], h[1]), st.Cost, mv)
		}
		if h[1] < 10 && h[0] > 7 && h[0] < 14 {
			// North of the river between the bridges: reachable only by crossing a river side.
			t.Errorf("reached %s across the river without a crossing", HexID(h[0], h[1]))
		}
	}
	if _, ok := reach[[2]int{u.Col, u.Row}]; !ok {
		t.Error("start hex missing from reach")
	}
	// A dropship stays on water.
	for h := range r.Reach(mod, r.Unit("d1")) {
		if !mod.TerrainAt(r.Map.TerrainCode(h[0], h[1])).Water() {
			t.Errorf("dropship reaches land at %s", HexID(h[0], h[1]))
		}
	}
}

func TestSaveCommitAndLockOrders(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1") // 1st Rifle Squad at 0608
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Risk: "balanced", Fallback: "hold", Commit: true}, now); err == nil {
		t.Error("committed a move with no destination")
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: "2020", Risk: "balanced", Fallback: "hold"}, now); err == nil {
		t.Error("saved an unreachable destination")
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m2", Task: "hold", Risk: "balanced", Fallback: "hold"}, now); err != ErrForbidden {
		t.Errorf("ordered someone else's unit: %v", err)
	}
	// Pick the farthest reachable hex east along the avenue.
	var dest [2]int
	for h, st := range r.Reach(mod, u) {
		if st.Cost > 0 && (h[0] > dest[0] || dest == [2]int{}) && h[1] == u.Row {
			dest = h
		}
	}
	in := OrderInput{Unit: "m1", Task: "move", Dest: HexID(dest[0], dest[1]), Intent: "Push toward the hospital", Risk: "balanced", Fallback: "hold"}
	o, err := r.SaveOrder(hicks, mod, in, now)
	if err != nil || o.Status != OrderDraft || o.Version != 1 || len(o.Route) == 0 {
		t.Fatalf("draft: %v %+v", err, o)
	}
	in.Commit = true
	if o2, _ := r.SaveOrder(hicks, mod, in, now); o2 != o || o.Status != OrderCommitted || o.Version != 2 || len(o.History) != 2 {
		t.Fatalf("commit revised a different order or lost history: %+v", o)
	}
	// Vasquez leaves a draft; the Weapons Squad gets nothing.
	r.SaveOrder(r.Member("v"), mod, OrderInput{Unit: "m2", Task: "hold", Risk: "balanced", Fallback: "hold"}, now)

	r.Tick(mod, r.Deadline)
	if o.Status != OrderLocked || r.CurrentOrder("m2").Status != OrderLapsed {
		t.Fatalf("after lock: m1 %s, m2 %s", o.Status, r.CurrentOrder("m2").Status)
	}
	if _, err := r.SaveOrder(hicks, mod, in, now); err == nil {
		t.Error("revised an order after lock")
	}
	var moved bool
	for _, m := range r.Resolution.Moves {
		moved = moved || m.Unit == "m1" && m.To() == dest
	}
	if !moved || !slices.ContainsFunc(r.Resolution.Notes, func(n string) bool { return strings.Contains(n, "2nd Rifle Squad") }) {
		t.Errorf("moves %+v, notes %v", r.Resolution.Moves, r.Resolution.Notes)
	}
	publishTurn(t, r, mod, ref, now)
	if u.Col != dest[0] || u.Row != dest[1] {
		t.Errorf("squad at %s, want %s", HexID(u.Col, u.Row), HexID(dest[0], dest[1]))
	}
}

func TestOrderedMoveStopsAtHiddenEnemy(t *testing.T) {
	r, mod, _, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	var dest [2]int
	var route [][2]int
	reach := r.Reach(mod, u)
	for h, st := range reach {
		if st.Cost > 0 {
			if p := routeTo(reach, [2]int{u.Col, u.Row}, h); len(p) >= 2 && len(p) > len(route) {
				dest, route = h, p
			}
		}
	}
	if len(route) < 2 {
		t.Skip("no multi-hex route from this start")
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(dest[0], dest[1]), Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	// An unseen horde slips onto the second hex of the route before lock.
	r.Units = append(r.Units, &Unit{ID: "ambush", Name: "Ambushers", Class: "crawlers", Side: "infected", Col: route[1][0], Row: route[1][1], Strength: 8})
	r.Tick(mod, r.Deadline)
	var fight *Encounter
	for _, e := range r.Resolution.Encounters {
		if slices.Contains(e.Friend, "m1") && slices.Contains(e.Enemy, "ambush") {
			fight = e
		}
	}
	if fight == nil || !fight.Sit[EnemySide].Unseen {
		t.Fatalf("no ambush encounter: %+v", r.Resolution.Encounters)
	}
	for _, m := range r.Resolution.Moves {
		if m.Unit == "m1" && slices.Contains(m.Path, route[1]) {
			t.Error("the squad walked through the ambushers")
		}
	}
}

func TestWithdrawOrder(t *testing.T) {
	r, mod, _, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	o, _ := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "cautious", Fallback: "hold", Commit: true}, now)
	if err := r.WithdrawOrder(r.Member("v"), mod, o.ID, now); err != ErrForbidden {
		t.Errorf("someone else withdrew it: %v", err)
	}
	if err := r.WithdrawOrder(hicks, mod, o.ID, now); err != nil || r.CurrentOrder("m1") != nil {
		t.Errorf("withdraw: %v", err)
	}
}
