package game

import "testing"

func TestCommanderReview(t *testing.T) {
	r, mod, _, cmd, hicks := orderRoom(t) // Hicks reports to Apone (cmd) and holds m1
	now := central(t, "2026-09-30 13:00")
	o, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "balanced", Fallback: "hold", Commit: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ReviewOrder(hicks, o.ID, "approve", "", now); err != ErrForbidden {
		t.Errorf("a player reviewed his own order: %v", err)
	}
	if err := r.ReviewOrder(r.Member("v"), o.ID, "approve", "", now); err != ErrForbidden {
		t.Errorf("another player reviewed it: %v", err)
	}
	if err := r.ReviewOrder(cmd, o.ID, "return", "", now); err == nil {
		t.Error("returned without saying why")
	}
	if err := r.ReviewOrder(cmd, o.ID, "return", "Hold the bridge instead", now); err != nil {
		t.Fatal(err)
	}
	if o.Status != OrderDraft || o.Review != ReviewReturned {
		t.Fatalf("returned order: %s %s", o.Status, o.Review)
	}
	// The player fixes and recommits; the review clears, and the commander approves.
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "hold", Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	if o.Review != "" {
		t.Error("a revision kept the old review")
	}
	if err := r.ReviewOrder(cmd, o.ID, "approve", "Good.", now); err != nil || o.Review != ReviewApproved || o.Status != OrderCommitted {
		t.Errorf("approve: %v %s %s", err, o.Review, o.Status)
	}
	// Off the net, the commander can't review.
	r.SetRadio(r.Referee(), mod, "m1", RadioDown)
	r.RefreshComms(mod)
	if r.CanReview(cmd, r.Unit("m1")) {
		t.Error("reviewed an order from a unit out of contact")
	}
}
