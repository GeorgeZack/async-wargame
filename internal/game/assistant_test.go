package game

import (
	"strings"
	"testing"
)

func TestAssistantCardsAndNarratives(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	var dest string
	for h, st := range r.Reach(mod, u) {
		if st.Cost > 0 && r.fits(mod, u, h, r.ProjectedLoad(mod, u.ID, nil)) {
			dest = HexID(h[0], h[1])
			break
		}
	}
	// A move with no intent: a rule card, even with the AI off.
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: dest, Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	var card *Issue
	for _, is := range r.Assistant.Issues {
		if is.Unit == "m1" && is.Source == "rules" && strings.Contains(is.Text, "no intent") {
			card = is
		}
	}
	if card == nil {
		t.Fatalf("no rule card: %+v", r.Assistant.Issues)
	}
	if len(r.Assistant.Runs) != 0 {
		t.Error("queued model work with the assistant off")
	}
	if err := r.DecideIssue(hicks, card.ID, "dismiss", "", now); err != ErrForbidden {
		t.Errorf("a player decided a card: %v", err)
	}
	// Forwarding it posts to Hicks's private channel.
	if err := r.DecideIssue(ref, card.ID, "send", "", now); err != nil || card.Status != IssueSent {
		t.Fatalf("send: %v, %s", err, card.Status)
	}
	if th := r.Thread(hicks, "ref-h"); len(th) != 1 || !strings.Contains(th[0].Text, "no intent") {
		t.Errorf("Hicks's private thread: %+v", th)
	}

	// A narrative stays a draft until approved; players don't see drafts.
	r.SetAssistant(ref, true, true, true)
	publishTurn(t, r, mod, ref, now)
	if len(r.Assistant.Runs) != 1 || r.Assistant.Runs[0].Kind != RunNarrative || r.Assistant.Runs[0].Turn != 1 {
		t.Fatalf("runs %+v", r.Assistant.Runs)
	}
	run := r.NextRun("big-pickle", "test", now)
	r.FinishRun(run.ID, 50, nil, func(x *Run) error { return r.ApplyNarrative(x, "Quiet night on the coast.") }, false, now)
	if v := BuildView(r, testScenario(t), mod, hicks); v.Reports[0].Narrative != "" {
		t.Error("a player saw a draft narrative")
	}
	if err := r.DecideNarrative(ref, 1, 1, "approve", "Quiet night on the coast. Mostly."); err != nil {
		t.Fatal(err)
	}
	v := BuildView(r, testScenario(t), mod, hicks)
	if v.Reports[0].Narrative != "Quiet night on the coast. Mostly." || !v.Reports[0].NarrativeEdited || v.Reports[0].NarrativeModel != "big-pickle" {
		t.Errorf("approved narrative: %+v", v.Reports[0])
	}
}
