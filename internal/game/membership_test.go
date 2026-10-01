package game

import "testing"

func TestLeavingHandsUnitsOn(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")
	o, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "cautious", Fallback: "hold"}, now)
	if err != nil {
		t.Fatal(err)
	}

	// Nobody else can remove Hicks; he can leave himself, and his units go to
	// his commander, draft and all.
	if err := r.Remove(vasq, hicks.ID, ""); err != ErrForbidden {
		t.Errorf("Vasquez removed Hicks: %v", err)
	}
	if err := r.Remove(hicks, hicks.ID, "v"); err != nil {
		t.Fatal(err)
	}
	if hicks.Status != StatusRemoved || r.Unit("m1").AssignedTo != cmd.ID {
		t.Errorf("after leaving: %s, m1 with %q", hicks.Status, r.Unit("m1").AssignedTo)
	}
	if r.CurrentOrder("m1") != o || !r.CanOrder(mod, cmd, r.Unit("m1")) {
		t.Error("Apone can't take over the squad's draft")
	}
	if err := r.Remove(hicks, hicks.ID, ""); err == nil {
		t.Error("left twice")
	}

	// The referee removes Apone and hands everything to Vasquez; Apone's
	// players would report to the referee.
	if err := r.Remove(ref, cmd.ID, "nobody"); err == nil {
		t.Error("handed units to someone who doesn't exist")
	}
	if err := r.Remove(ref, cmd.ID, vasq.ID); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").AssignedTo != vasq.ID {
		t.Errorf("m1 with %q, want Vasquez", r.Unit("m1").AssignedTo)
	}
	// Removing Vasquez back to the referee leaves the units unassigned.
	if err := r.Remove(ref, vasq.ID, "referee"); err != nil || r.Unit("m1").AssignedTo != "" || r.Unit("m2").AssignedTo != "" {
		t.Errorf("back to the referee: %v, m1 %q m2 %q", err, r.Unit("m1").AssignedTo, r.Unit("m2").AssignedTo)
	}
	if err := r.Remove(ref, ref.ID, ""); err == nil {
		t.Error("the referee removed themselves")
	}
}
