package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func autoRoom(t *testing.T) (*Room, *Module, *Member) {
	t.Helper()
	r, mod, ref := startedRoom(t, time.Now())
	if err := r.SetAutomatic(ref, "enable", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveOrder(ref, mod, "m1", "Hold the line.", nil, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.LockNow(ref, mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	return r, mod, ref
}
func autoFinish(t *testing.T, r *Room, mod *Module, reply string, wantErr bool) *Run {
	t.Helper()
	run := r.NextRun("test-model", "mock", time.Now())
	if run == nil {
		t.Fatal("no pending run")
	}
	r.FinishRun(run.ID, 20, nil, func(stored *Run) error { return r.ApplyReply(mod, stored, reply, jsonDecode) }, false, time.Now())
	var finished *Run
	for _, x := range r.Assistant.Runs {
		if x.ID == run.ID {
			finished = x
		}
	}
	if finished == nil {
		t.Fatal("lost run")
	}
	if (finished.Status == RunFailed) != wantErr {
		t.Fatalf("%s: status=%s error=%s", run.Kind, finished.Status, finished.Error)
	}
	return finished
}
func autoIntentReply(r *Room, mod *Module) string {
	var out []map[string]string
	for _, u := range r.Units {
		if mod.Side(u.Side).Enemy && !u.Destroyed() {
			out = append(out, map[string]string{"unit": u.ID, "intent": "Wait for noise."})
		}
	}
	b, _ := json.Marshal(map[string]any{"intentions": out})
	return string(b)
}
func autoToResolution(t *testing.T, r *Room, mod *Module) {
	autoFinish(t, r, mod, autoIntentReply(r, mod), false)
	autoFinish(t, r, mod, `{"contests":[]}`, false)
}
func autoDecisionReply(r *Room) string {
	var outcomes []AutoOutcome
	for _, o := range r.LockedOrders() {
		outcomes = append(outcomes, AutoOutcome{Order: o.ID, Title: "Holding", Text: "The unit held position.", Seen: SeenByAll})
	}
	b, _ := json.Marshal(AutoDecision{Outcomes: outcomes, Reason: "No opposition challenged the position."})
	return string(b)
}

func TestAutomaticDecisionCanUpdateMoreThanTwoHundredUnits(t *testing.T) {
	r, mod, _ := autoRoom(t)
	var d AutoDecision
	if err := json.Unmarshal([]byte(autoDecisionReply(r)), &d); err != nil {
		t.Fatal(err)
	}
	status := "Holding the sector"
	for i := 0; i < 201; i++ {
		u := *r.Unit("m1")
		u.ID, u.Formation, u.AssignedTo = fmt.Sprintf("extra-%d", i), "", ""
		r.Units = append(r.Units, &u)
		d.Units = append(d.Units, AutoUnitEdit{Unit: u.ID, Status: &status})
	}
	if err := r.applyAutoDecision(mod, &d); err != nil {
		t.Fatal(err)
	}
	if r.Unit("extra-200").Status != status {
		t.Fatal("last unit's update was lost")
	}
	d.Units = append(d.Units, d.Units[0])
	if err := r.cloneAuto().applyAutoDecision(mod, &d); err == nil {
		t.Fatal("duplicate unit edit accepted")
	}
}

func TestAutomaticRefereePublishesAndCorrectionPauses(t *testing.T) {
	r, mod, ref := autoRoom(t)
	before := r.Unit("m1").Pos()
	autoToResolution(t, r, mod)
	var d AutoDecision
	json.Unmarshal([]byte(autoDecisionReply(r)), &d)
	p := Pos{before.X + 1, before.Y}
	detached := true
	status := "Winded"
	d.Units = []AutoUnitEdit{{Unit: "m1", At: &p, Detached: &detached, Status: &status}}
	b, _ := json.Marshal(d)
	autoFinish(t, r, mod, string(b), false)
	if r.Unit("m1").Pos() != before || r.Unit("m1").Status == status || len(r.Reports) != 0 {
		t.Fatal("decision leaked before publication")
	}
	// Proposal survives restart, along with the queued publication.
	b, _ = json.Marshal(r)
	var restored Room
	json.Unmarshal(b, &restored)
	r = &restored
	autoFinish(t, r, mod, "The squad moved forward.", false)
	if r.Turn != 2 || r.Phase != PhasePlanning || len(r.Reports) != 1 || r.Unit("m1").Pos() != p || r.Unit("m1").Status != status || !r.Unit("m1").Detached {
		t.Fatalf("publication failed: turn %d", r.Turn)
	}
	if r.Reports[0].NarrativeStatus != NarrativeApproved || len(r.Plans) != 0 || r.Assistant.Decision != nil || r.HasPendingRun() {
		t.Fatal("unfinished publication")
	}
	if err := r.ReopenTurn(ref, "Correct the movement", time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.Assistant.Automatic || r.Unit("m1").Pos() != before || r.Unit("m1").Detached || r.Phase != PhaseResolving {
		t.Fatal("correction did not restore pre-AI state and pause")
	}
}

func TestAutomaticRejectsInvalidDecisionsAtomically(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*AutoDecision, *Room)
	}{
		{"missing order", func(d *AutoDecision, r *Room) { d.Outcomes = nil }},
		{"duplicate order", func(d *AutoDecision, r *Room) { d.Outcomes = append(d.Outcomes, d.Outcomes[0]) }},
		{"unknown unit", func(d *AutoDecision, r *Room) { d.Units = []AutoUnitEdit{{Unit: "ghost"}} }},
		{"invalid status after valid casualty", func(d *AutoDecision, r *Room) {
			n := 0
			s := strings.Repeat("x", 201)
			d.Units = []AutoUnitEdit{{Unit: "m1", Strength: &n}, {Unit: "m2", Status: &s}}
		}},
		{"different attached destinations", func(d *AutoDecision, r *Room) {
			p := r.Unit("m1").Pos()
			p.X++
			d.Units = []AutoUnitEdit{{Unit: "m1", At: &p}}
		}},
		{"bad audience", func(d *AutoDecision, r *Room) { d.Outcomes[0].Seen = "everyone" }},
		{"private event without holder", func(d *AutoDecision, r *Room) {
			d.Outcomes = append(d.Outcomes, AutoOutcome{Title: "Secret", Text: "A sound.", Seen: SeenByHolder})
		}},
		{"invalid threat", func(d *AutoDecision, r *Room) { n := 6; d.Threat = &n }},
		{"reinforcements", func(d *AutoDecision, r *Room) {
			n := r.Unit("m1").Strength + 1
			d.Units = []AutoUnitEdit{{Unit: "m1", Strength: &n}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mod, _ := autoRoom(t)
			autoToResolution(t, r, mod)
			var d AutoDecision
			json.Unmarshal([]byte(autoDecisionReply(r)), &d)
			tc.change(&d, r)
			before := r.autoFingerprint()
			b, _ := json.Marshal(d)
			autoFinish(t, r, mod, string(b), true)
			if r.autoFingerprint() != before || r.HasPendingRun() || len(r.Reports) > 0 {
				t.Fatal("failed validation changed game or continued")
			}
		})
	}
}

func TestAutomaticPauseStaleResponseAndRetry(t *testing.T) {
	t.Run("pause", func(t *testing.T) {
		r, mod, ref := autoRoom(t)
		run := r.NextRun("model", "mock", time.Now())
		r.SetAutomatic(ref, "pause", time.Now())
		r.FinishRun(run.ID, 10, nil, func(x *Run) error { return r.ApplyReply(mod, x, autoIntentReply(r, mod), jsonDecode) }, false, time.Now())
		if r.HasPendingRun() || len(r.Intentions) > 0 || r.Assistant.Automatic {
			t.Fatal("late response applied")
		}
	})
	t.Run("human edit", func(t *testing.T) {
		r, mod, _ := autoRoom(t)
		run := r.NextRun("model", "mock", time.Now())
		r.RefereeNotes = "Changed situation"
		err := r.ApplyReply(mod, run, autoIntentReply(r, mod), jsonDecode)
		if err == nil || len(r.Intentions) > 0 {
			t.Fatal("stale reply applied")
		}
	})
	t.Run("retry preserves dice", func(t *testing.T) {
		r, mod, ref := autoRoom(t)
		autoFinish(t, r, mod, autoIntentReply(r, mod), false)
		reply := fmt.Sprintf(`{"contests":[{"order":%q,"title":"Listen","a":{"unit":"m1"}}]}`, r.LockedOrders()[0].ID)
		autoFinish(t, r, mod, reply, false)
		ct := *r.Desk.Contests[0]
		autoFinish(t, r, mod, `{}`, true)
		if err := r.SetAutomatic(ref, "retry", time.Now()); err != nil {
			t.Fatal(err)
		}
		autoFinish(t, r, mod, autoDecisionReply(r), false)
		if len(r.Desk.Contests) != 1 || *r.Desk.Contests[0].Result != *ct.Result {
			t.Fatal("retry rerolled dice")
		}
	})
	t.Run("edit between resolution and briefing", func(t *testing.T) {
		r, mod, ref := autoRoom(t)
		autoToResolution(t, r, mod)
		autoFinish(t, r, mod, autoDecisionReply(r), false)
		r.Unit("m1").Status = "Human change"
		autoFinish(t, r, mod, "Old briefing", true)
		if err := r.SetAutomatic(ref, "retry", time.Now()); err != nil {
			t.Fatal(err)
		}
		if run := r.NextRun("m", "p", time.Now()); run.Kind != RunAutoResolve {
			t.Fatal("retry used stale decision")
		}
	})
}

func TestAutomaticPrivacyAndAuthorization(t *testing.T) {
	r, mod, ref := autoRoom(t)
	player := &Member{ID: "player", Role: RolePlayer}
	if err := r.SetAutomatic(player, "enable", time.Now()); err != ErrForbidden {
		t.Fatal("player enabled AI")
	}
	_, prompt, _ := r.autoPrompt(mod, &Run{Kind: RunAutoIntentions, Turn: r.Turn, Operation: 1})
	if strings.Contains(prompt, "Hold the line") {
		t.Fatal("enemy intentions saw player orders")
	}
	autoToResolution(t, r, mod)
	r.Unit("m1").OutOfContact = true
	var d AutoDecision
	json.Unmarshal([]byte(autoDecisionReply(r)), &d)
	d.Outcomes[0].Text = "PRIVATE discovery"
	d.Outcomes = append(d.Outcomes, AutoOutcome{Title: "Enemy", Text: "SECRET trap", Seen: SeenByReferee})
	b, _ := json.Marshal(d)
	autoFinish(t, r, mod, string(b), false)
	_, prompt, ok := r.autoPrompt(mod, &Run{Kind: RunAutoPublish, Turn: r.Turn, Operation: 1})
	if !ok || strings.Contains(prompt, "PRIVATE") || strings.Contains(prompt, "SECRET") {
		t.Fatal("briefing prompt leaked private outcomes", prompt)
	}
	autoFinish(t, r, mod, "Orders are awaited.", false)
	if r.Reports[0].Entries[0].Seen != SeenByHolder {
		t.Fatal("out of contact outcome was public")
	}
	// Future deadlines keep the full automatic cycle enabled.
	r.Tick(mod, r.Deadline)
	if !r.HasPendingRun() {
		t.Fatal("next scheduled turn did not start automatically")
	}
	r.SetAutomatic(ref, "pause", time.Now())
}

func TestLimitedContactRemainsPrivateAndNarrationUsesRecordedMovement(t *testing.T) {
	r, mod, _ := autoRoom(t)
	hidden := r.Unit("z3")
	hidden.Known = false
	hidden.KnownTo = []string{"one-player-only"}
	if !r.promptUnit(mod, hidden).Hidden {
		t.Fatal("one-player discovery treated as globally public")
	}
	after := r.cloneAuto()
	after.Unit("m1").X += 1
	after.Unit("m1").Strength = 0
	after.Unit("z3").X += 1
	after.Unit("z3").Strength = 0
	facts := strings.Join(r.publicDecisionFacts(mod, after), "\n")
	if !strings.Contains(facts, "Recorded movement") || !strings.Contains(facts, "is destroyed") {
		t.Fatal("missing authoritative narrative facts")
	}
	if strings.Contains(facts, hidden.Name) {
		t.Fatal("private discovery leaked into public narrator")
	}
	r.Unit("m1").OutOfContact = true
	if strings.Contains(strings.Join(r.publicDecisionFacts(mod, after), "\n"), r.Unit("m1").Name) {
		t.Fatal("stale contact revealed new location")
	}
}

func TestAutomaticRetryBoundAndStaleEdits(t *testing.T) {
	r, mod, _ := autoRoom(t)
	autoToResolution(t, r, mod)
	failed := autoFinish(t, r, mod, `{"outcomes":[],"reason":"incomplete"}`, true)
	if !r.RetryAutomatic(failed.ID, 1, time.Now()) {
		t.Fatal("transient failure wasn't retried")
	}
	failed = autoFinish(t, r, mod, `{"outcomes":[],"reason":"still incomplete"}`, true)
	if failed.Attempt != 1 || r.RetryAutomatic(failed.ID, 1, time.Now()) {
		t.Fatal("unbounded automatic retry")
	}
	if err := r.SetAutomatic(r.Referee(), "retry", time.Now()); err != nil {
		t.Fatal(err)
	}
	run := r.NextRun("m", "p", time.Now())
	r.Unit("m1").Status = "host edit"
	r.FinishRun(run.ID, 0, fmt.Errorf("provider failure"), nil, false, time.Now())
	if r.RetryAutomatic(run.ID, 10, time.Now()) {
		t.Fatal("automatic retry ignored a host edit")
	}
}

func TestPrivateContactContestIsNotPublic(t *testing.T) {
	r, mod, _ := autoRoom(t)
	u := r.Unit("z3")
	u.Known = false
	u.KnownTo = []string{"only-one-player"}
	autoFinish(t, r, mod, autoIntentReply(r, mod), false)
	autoFinish(t, r, mod, `{"contests":[{"order":"","title":"Private sighting","a":{"unit":"m1"},"b":{"unit":"z3"},"secret":false}]}`, false)
	if len(r.Desk.Contests) != 1 || !r.Desk.Contests[0].Secret {
		t.Fatal("limited discovery contest leaked")
	}
}

func TestNewRadioLossKeepsMovementAndOutcomePrivate(t *testing.T) {
	r, mod, _ := autoRoom(t)
	r.RefreshComms(mod)
	before := r.Unit("m1").Pos()
	autoToResolution(t, r, mod)
	var d AutoDecision
	json.Unmarshal([]byte(autoDecisionReply(r)), &d)
	at := Pos{before.X + 1, before.Y}
	yes := true
	d.Units = []AutoUnitEdit{{Unit: "m1", At: &at, Detached: &yes, RadioDown: &yes}}
	raw, _ := json.Marshal(d)
	autoFinish(t, r, mod, string(raw), false)
	if r.Assistant.Decision.Outcomes[0].Seen != SeenByHolder {
		t.Fatal("new radio loss left outcome public")
	}
	preview := r.cloneAuto()
	if err := preview.applyAutoDecision(mod, preview.Assistant.Decision); err != nil {
		t.Fatal(err)
	}
	if !preview.Unit("m1").OutOfContact || *preview.Unit("m1").Reported != before {
		t.Fatal("lost last reported position")
	}
	if strings.Contains(strings.Join(r.publicDecisionFacts(mod, preview), "\n"), r.Unit("m1").Name) {
		t.Fatal("radio-loss movement leaked to narrator")
	}
}
