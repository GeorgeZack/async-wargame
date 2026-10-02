package game

import (
	"strings"
	"testing"
)

func TestEnemyIntentions(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	if err := r.SetIntention(hicks, mod, "z1", "Eat Hicks."); err != ErrForbidden {
		t.Errorf("a player set an intention: %v", err)
	}
	if err := r.SetIntention(ref, mod, "m1", "Desert."); err == nil {
		t.Error("set an intention for a marine")
	}
	if err := r.SetIntention(ref, mod, "z1", "Drift toward the radio noise at the beach."); err != nil {
		t.Fatal(err)
	}
	// Only the referee sees intentions.
	if v := BuildView(r, testScenario(t), mod, hicks); v.Intentions != nil {
		t.Error("a player sees intentions")
	}
	v := BuildView(r, testScenario(t), mod, ref)
	found := false
	for _, iv := range v.Intentions {
		found = found || iv.Unit.ID == "z1" && iv.Text == "Drift toward the radio noise at the beach."
	}
	if !found || len(v.Intentions) != 4 {
		t.Errorf("referee intentions: %+v", v.Intentions)
	}
	// They reach the assistant's prompts, and survive into judging.
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes crosses the bridge.", "", true, now)
	r.Tick(mod, r.Deadline)
	_, user, ok := r.PromptFor(mod, &Run{Kind: RunOutcome, Target: o.ID, Turn: r.Turn})
	if ok && !strings.Contains(user, "Drift toward the radio noise") && strings.Contains(user, `"id": "z1"`) {
		t.Errorf("outcome prompt lacks the intention: %s", user)
	}
	_, user, ok = r.PromptFor(mod, &Run{Kind: RunIntentions, Turn: r.Turn})
	if !ok || !strings.Contains(user, "currentIntention") || strings.Contains(user, "Reyes crosses the bridge") {
		t.Errorf("intentions prompt: ok %v, %s", ok, user)
	}
	// The assistant's proposals don't overwrite the referee's.
	if err := r.applyIntentions(mod, &Run{Turn: r.Turn, Model: "m"}, `{"intentions":[{"unit":"z1","intent":"Sleep."},{"unit":"z2","intent":"Bang on the pharmacy shutters."},{"unit":"m1","intent":"Mutiny."}]}`, decodeTest); err != nil {
		t.Fatal(err)
	}
	if r.IntentionOf("z1") != "Drift toward the radio noise at the beach." || r.IntentionOf("z2") != "Bang on the pharmacy shutters." || r.IntentionOf("m1") != "" {
		t.Errorf("after AI: z1 %q z2 %q m1 %q", r.IntentionOf("z1"), r.IntentionOf("z2"), r.IntentionOf("m1"))
	}
	// Next turn starts clean.
	publishTurn(t, r, mod, ref, now)
	if r.IntentionOf("z1") != "" {
		t.Error("an intention carried into the next turn")
	}
}

func TestAskingAPlayerAboutAnOrder(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes goes in and deals with it.", "", true, now)
	if err := r.AskAboutOrder(ref, o.ID, "Deal with what?", now); err == nil {
		t.Error("asked before the lock")
	}
	r.Tick(mod, r.Deadline)
	if err := r.AskAboutOrder(hicks, o.ID, "Hmm?", now); err != ErrForbidden {
		t.Errorf("a player asked: %v", err)
	}
	if err := r.applyQuestion(&Run{Turn: r.Turn, Target: o.ID, Model: "m"}, `"Go in where: the side door or the lobby?"`); err != nil {
		t.Fatal(err)
	}
	if q := r.Desk.Questions[o.ID]; q == nil || q.Text != "Go in where: the side door or the lobby?" || q.Source != "ai" {
		t.Fatalf("draft %+v", q)
	}
	if err := r.AskAboutOrder(ref, o.ID, "Go in where: the side door or the lobby?", now); err != nil {
		t.Fatal(err)
	}
	th := r.Thread(hicks, "ref-h")
	if len(th) != 1 || !strings.Contains(th[0].Text, "About Sgt. Reyes's order this turn: Go in where") || r.Desk.Questions[o.ID].Sent.IsZero() {
		t.Errorf("thread %+v", th)
	}
}

func decodeTest(reply string, v any) error {
	return jsonDecode(reply, v)
}
