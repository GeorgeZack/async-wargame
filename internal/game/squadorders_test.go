package game

import (
	"strings"
	"testing"
	"time"
)

func TestSquadOrderOneThroughFourInstructions(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	for _, u := range r.members("lantern") {
		u.AssignedTo = player.ID
	}
	now := time.Now()
	if _, err := r.SaveOrder(player, mod, "f-lantern", "Squad advances to the bridge", nil, true, now); err != nil {
		t.Fatal(err)
	}
	if in, total, _ := r.OrderReadiness(); in != 4 || total != 4 {
		t.Fatalf("readiness %d/%d", in, total)
	}
	v := BuildView(r, testScenario(t), mod, player)
	if len(v.SquadOrders) != 1 || v.SquadOrders[0].Order == nil || !strings.Contains(v.SquadOrders[0].Order.Members, "Doyle") {
		t.Fatal("missing squad view")
	}
	for i, id := range []string{"m2", "m3", "m4"} {
		if _, err := r.SaveOrder(player, mod, id, "Scout separately", nil, true, now); err != nil {
			t.Fatal(err)
		}
		r.lockOrders()
		if got := len(r.LockedOrders()); got != i+2 {
			t.Fatalf("expected %d effective orders, got %d", i+2, got)
		}
		for _, o := range r.Orders {
			if o.Status == OrderLocked {
				o.Status = OrderCommitted
			}
		}
	}
	// Updating the global instruction preserves the three individual exceptions.
	r.SaveOrder(player, mod, "f-lantern", "Hold the bridge", nil, true, now)
	if r.CurrentOrder("m2").Text != "Scout separately" || r.CurrentOrder("m1").Text != "Hold the bridge" {
		t.Fatal("global order overwrote individual exceptions")
	}
	for _, id := range []string{"m2", "m3", "m4"} {
		if err := r.WithdrawOrder(player, mod, r.CurrentOrder(id).ID, now); err != nil {
			t.Fatal(err)
		}
		if r.CurrentOrder(id).Group != "lantern" {
			t.Fatal("withdraw did not rejoin shared instruction")
		}
	}
	// Planning a squad drag preserves a single shared order; one member's plan becomes an exception.
	var moves []UnitMove
	for _, u := range r.members("lantern") {
		moves = append(moves, UnitMove{u.ID, Pos{200, 150}})
	}
	if err := r.PlanUnits(player, mod, moves, now); err != nil {
		t.Fatal(err)
	}
	for _, u := range r.members("lantern") {
		if r.CurrentOrder(u.ID).Group != "lantern" {
			t.Fatal("squad planning fragmented order")
		}
	}
	if err := r.PlanUnits(player, mod, []UnitMove{{"m4", Pos{250, 150}}}, now); err != nil {
		t.Fatal(err)
	}
	if r.CurrentOrder("m4").Group != "" {
		t.Fatal("individual planning changed the global pin")
	}
	r.Tick(mod, r.Deadline)
	if len(r.LockedOrders()) != 2 {
		t.Fatal("desk should have one squad and one individual instruction")
	}
	desk := BuildView(r, testScenario(t), mod, ref).Desk
	if len(desk.Items) != 2 || len(desk.Missing) != 0 {
		t.Fatalf("incorrect desk: %+v", desk)
	}
	for _, o := range r.LockedOrders() {
		r.SetOutcome(ref, o.ID, "", "", "Completed", SeenByHolder)
	}
	if err := r.PublishTurn(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	if len(r.Reports[0].Entries) != 2 {
		t.Fatal("shared order published multiple times")
	}
	if len(BuildView(r, testScenario(t), mod, player).Reports[0].Entries) != 2 {
		t.Fatal("member cannot see private shared result")
	}
}

func TestSquadOrderPermissions(t *testing.T) {
	r, mod, ref, cmd, player := orderRoom(t)
	now := time.Now()
	if _, err := r.SaveOrder(player, mod, "f-lantern", "Take the bridge", nil, true, now); err != ErrForbidden {
		t.Fatal("player ordered other members")
	}
	if _, err := r.SaveOrder(cmd, mod, "f-lantern", "Take the bridge", nil, true, now); err != ErrForbidden {
		t.Fatal("commander ordered units outside their command")
	}
	// Referee admin mode can issue the shared instruction, but an individual
	// holder must not be able to withdraw the entire group through its order ID.
	ref.AdminMode = true
	o, err := r.SaveOrder(ref, mod, "f-lantern", "Take the bridge", nil, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.WithdrawOrder(player, mod, o.ID, now); err != ErrForbidden {
		t.Fatal("individual withdrew whole group")
	}
	if _, err := r.SaveOrder(player, mod, "m1", "Scout alone", nil, true, now); err != nil {
		t.Fatal(err)
	}
	if r.CurrentOrder("m1").Group != "" || r.CurrentOrder("m2").Group != "lantern" {
		t.Fatal("individual override altered peers")
	}
}
