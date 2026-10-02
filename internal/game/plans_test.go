package game

import (
	"math"
	"testing"
)

func TestPlansStayPrivateAndUpdateOrders(t *testing.T) {
	r, mod, ref, cmd, player := orderRoom(t)
	now := central(t, "2026-10-01 12:00")
	start := r.Unit("m1").Pos()
	at := Pos{200, 150}
	if err := r.PlanUnits(player, mod, []UnitMove{{"m1", at}}, now); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != start || len(r.Moves) != 0 {
		t.Fatal("planning moved real units")
	}
	for _, me := range []*Member{ref, cmd, r.Member("v")} {
		if len(BuildView(r, testScenario(t), mod, me).Data.Plans) != 0 {
			t.Fatalf("%s sees private plans", me.Name)
		}
	}
	if len(BuildView(r, testScenario(t), mod, player).Data.Plans) != 1 {
		t.Fatal("own plan missing")
	}
	o, err := r.SaveOrder(player, mod, "m1", "Move to the bridge", &at, true, now)
	if err != nil {
		t.Fatal(err)
	}
	at = Pos{240, 180}
	if err := r.PlanUnits(player, mod, []UnitMove{{"m1", at}}, now); err != nil {
		t.Fatal(err)
	}
	if o.Mark == nil || *o.Mark != at || o.Status != OrderCommitted || o.Version != 2 {
		t.Fatalf("plan did not update order: %+v", o)
	}
	if err := r.PlanUnits(player, mod, []UnitMove{{"m1", Pos{1, 1}}, {"m2", at}}, now); err != ErrForbidden {
		t.Fatalf("foreign unit planned: %v", err)
	}
	if r.PlansFor(player)[0].To != at {
		t.Fatal("invalid batch partially applied")
	}
	if err := r.ApplyPlans(player, mod); err != ErrForbidden {
		t.Fatal("player applied plans")
	}
	if err := r.PlanUnits(ref, mod, []UnitMove{{"m1", Pos{100, 100}}, {"z1", Pos{300, 200}}}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyPlans(ref, mod); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m2").Pos() != (Pos{100, 100}) || r.Unit("z1").Pos() != (Pos{300, 200}) || len(r.PlansFor(ref)) != 0 || len(r.PlansFor(player)) != 1 {
		t.Fatal("apply did not move formation and clear only referee plans")
	}
	r.Tick(mod, r.Deadline)
	if err := r.PlanUnits(player, mod, []UnitMove{{"m1", at}}, now); err != ErrForbidden {
		t.Fatal("player planned during resolution")
	}
	publishTurn(t, r, mod, ref, now)
	if len(r.PlansFor(player)) != 0 {
		t.Fatal("old turn plans visible")
	}
	if err := r.ReopenTurn(ref, "Correct position", now); err != nil {
		t.Fatal(err)
	}
	if len(r.PlansFor(player)) != 1 {
		t.Fatal("correction did not restore plans")
	}
}

func TestMoveBatchValidation(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	start := r.Unit("m1").Pos()
	for _, moves := range [][]UnitMove{{{"m1", Pos{1, 1}}, {"missing", Pos{2, 2}}}, {{"m1", Pos{math.NaN(), 2}}}} {
		if err := r.MoveUnits(ref, mod, moves); err == nil {
			t.Fatal("invalid move accepted")
		}
		if r.Unit("m1").Pos() != start {
			t.Fatal("invalid batch changed unit")
		}
	}
}

func TestPublishFinalizesRefereePlansAndCorrectionRestoresThem(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	now := central(t, "2026-10-01 12:00")
	start := r.Unit("m1").Pos()
	r.PlanUnits(player, mod, []UnitMove{{"m1", Pos{300, 200}}}, now)
	r.Tick(mod, r.Deadline)
	at := Pos{150, 100}
	if err := r.PlanUnits(ref, mod, []UnitMove{{"m1", at}}, now); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != start {
		t.Fatal("staging applied before publish")
	}
	// Moving another attached member updates the whole formation's destination.
	at = Pos{180, 120}
	if err := r.PlanUnits(ref, mod, []UnitMove{{"m2", at}}, now); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.PlansFor(ref) {
		if p.To != at {
			t.Fatal("conflicting staged formation destinations")
		}
	}
	if err := r.PublishTurn(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	for _, u := range r.members("lantern") {
		if u.Pos() != at {
			t.Fatal("publishing did not finalize staged positions")
		}
	}
	if len(r.Plans) != 0 {
		t.Fatal("temporary plans remained after publish")
	}
	if err := r.ReopenTurn(ref, "Correct the destination", now); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != start || len(r.PlansFor(ref)) != 4 || len(r.PlansFor(player)) != 1 {
		t.Fatal("correction did not restore pending plans and old positions")
	}
	if err := r.PublishTurn(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != at {
		t.Fatal("republish lost staged move")
	}
}
