package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func batchRoom(t *testing.T) (*Room, *Module) {
	r, mod, ref := autoRoom(t)
	// More independent units than one batch, plus the existing four-member squad.
	for i := 0; i < 25; i++ {
		u := *r.Unit("m1")
		u.ID = fmt.Sprintf("load-%02d", i)
		u.Name = u.ID
		u.Formation = ""
		u.AssignedTo = ""
		u.X += float64(i)
		r.Units = append(r.Units, &u)
	}
	r.Assistant.Batches = nil
	r.prepareAutoBatches()
	if len(r.Assistant.Batches.Groups) < 3 {
		t.Fatal("fixture did not batch")
	}
	_ = ref
	return r, mod
}
func TestBatchOwnershipAndAtomicMerge(t *testing.T) {
	r, mod := batchRoom(t)
	p := r.Assistant.Batches
	// Existing formations are never split between writers.
	for _, group := range p.Groups {
		n := 0
		for _, id := range group {
			if r.Unit(id).Formation == "lantern" {
				n++
			}
		}
		if n != 0 && n != 4 {
			t.Fatal("split formation ownership")
		}
	}
	before := r.Unit("load-00").Strength
	for i := range p.Groups {
		p.Index = i
		d := AutoDecision{Reason: "Held position.", Model: "test"}
		for _, o := range r.autoBatchOrders() {
			d.Outcomes = append(d.Outcomes, AutoOutcome{Order: o.ID, Title: "Hold", Text: "Held.", Seen: SeenByAll})
		}
		dead := 0
		for _, id := range p.Groups[i] {
			if id == "load-00" {
				d.Units = append(d.Units, AutoUnitEdit{Unit: id, Strength: &dead})
			}
		}
		if err := r.validateBatchDecision(mod, &d); err != nil {
			t.Fatal(err)
		}
		foreign := ""
		for _, u := range r.Units {
			if !r.autoOwnsUnit(u.ID) {
				foreign = u.ID
				break
			}
		}
		bad := d
		bad.Units = append([]AutoUnitEdit{}, d.Units...)
		bad.Units = append(bad.Units, AutoUnitEdit{Unit: foreign, Strength: &dead})
		if err := r.validateBatchDecision(mod, &bad); err == nil {
			t.Fatal("foreign unit write accepted")
		}
		p.Decisions = append(p.Decisions, d)
	}
	if err := r.mergeAutoBatches(mod); err != nil {
		t.Fatal(err)
	}
	if r.Unit("load-00").Strength != before {
		t.Fatal("partial decision became authoritative")
	}
	if err := r.cloneAuto().applyAutoDecision(mod, r.Assistant.Decision); err != nil {
		t.Fatal(err)
	}
}
func TestBatchRestartAndStaleRetry(t *testing.T) {
	r, mod := batchRoom(t)
	r.Assistant.Batches.Index = 1
	r.Assistant.Batches.Decisions = []AutoDecision{{Reason: "checkpoint"}}
	r.Assistant.Runs = nil
	r.Assistant.queueRun(RunAutoResolve, "batch-2", r.Operation, r.Turn, time.Now())
	run := r.NextRun("test", "test", time.Now())
	data, _ := json.Marshal(r)
	var resumed Room
	if err := json.Unmarshal(data, &resumed); err != nil {
		t.Fatal(err)
	}
	if !resumed.ResetRunning() || resumed.Assistant.Batches.Index != 1 || len(resumed.Assistant.Batches.Decisions) != 1 {
		t.Fatal("restart lost checkpoint")
	}
	r.Unit("load-00").Status = "changed by host"
	if _, _, ok := r.PromptFor(mod, run); ok {
		t.Fatal("stale checkpoint used")
	}
	r.FinishRun(run.ID, 0, fmt.Errorf("stale"), nil, false, time.Now())
	if err := r.SetAutomatic(r.Referee(), "retry", time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.Assistant.Batches != nil || r.Assistant.Decision != nil {
		t.Fatal("retry retained stale proposals")
	}
	next := r.NextRun("test", "test", time.Now())
	if next.Kind != RunAutoIntentions {
		t.Fatal("stale retry skipped replanning")
	}
}
func TestBatchPromptsHaveOtherOrdersButEnemyCannotReadThem(t *testing.T) {
	r, mod := batchRoom(t)
	run := r.NextRun("test", "test", time.Now())
	_, enemy, _ := r.PromptFor(mod, run)
	if strings.Contains(enemy, "Hold the line.") {
		t.Fatal("enemy planner read orders")
	}
	run.Kind = RunAutoResolve
	_, body, ok := r.PromptFor(mod, run)
	if !ok || !strings.Contains(body, "otherBatches") {
		t.Fatal("missing simultaneous context")
	}
}

func TestRepairSplitKeepsCompletedBatchesAndFormationOwnership(t *testing.T) {
	r, _ := batchRoom(t)
	p := r.Assistant.Batches
	p.Index = 1
	p.Decisions = []AutoDecision{{Reason: "Already resolved, remains private"}}
	first := append([]string(nil), p.Groups[0]...)
	before := r.autoFingerprint()
	originalCount := len(p.Groups)
	if !r.splitFailedAutoBatch() || len(p.Groups) <= originalCount || p.Index != 1 || len(p.Decisions) != 1 {
		t.Fatal("repair discarded completed work")
	}
	if strings.Join(first, ",") != strings.Join(p.Groups[0], ",") || r.autoFingerprint() != before {
		t.Fatal("repair changed authoritative game state")
	}
	seen := map[string]int{}
	formations := map[string]int{}
	for i, g := range p.Groups {
		for _, id := range g {
			seen[id]++
			u := r.Unit(id)
			if u.Formation != "" {
				if prior, ok := formations[u.Formation]; ok && prior != i {
					t.Fatal("repair split a formation")
				}
				formations[u.Formation] = i
			}
		}
	}
	for _, u := range r.Units {
		if seen[u.ID] != 1 {
			t.Fatal("repair lost/duplicated a unit")
		}
	}
}

func TestBatchContradictoryOutcomeIsRegeneratedFromMergedState(t *testing.T) {
	r, mod := batchRoom(t)
	p := r.Assistant.Batches
	// A batch claims an enemy retreated, while the enemy's writer holds it.
	enemy := r.Unit("z1")
	if enemy == nil {
		t.Fatal("missing enemy fixture")
	}
	before := enemy.Pos()
	for i := range p.Groups {
		p.Index = i
		d := AutoDecision{Reason: "Resolve this ownership batch."}
		for _, o := range r.autoBatchOrders() {
			d.Outcomes = append(d.Outcomes, AutoOutcome{Order: o.ID, Title: "Exchange", Text: enemy.Name + " broke east.", Seen: SeenByReferee})
		}
		p.Decisions = append(p.Decisions, d)
	}
	if err := r.mergeAutoBatches(mod); err != nil {
		t.Fatal(err)
	}
	r.Assistant.Runs = nil
	r.Assistant.queueRun(RunAutoResolve, "merged-outcomes", r.Operation, r.Turn, time.Now())
	run := r.NextRun("mock", "mock", time.Now())
	system, user, ok := r.PromptFor(mod, run)
	if !ok || !strings.Contains(system, "Rewrite every outcome") || !strings.Contains(user, "mergedState") || !strings.Contains(user, "broke east") {
		t.Fatal("contradictory draft was not sent for regeneration against merged state")
	}
	var out []AutoOutcome
	for _, o := range r.LockedOrders() {
		out = append(out, AutoOutcome{Order: o.ID, Title: "Exchange", Text: "Both sides held position.", Seen: SeenByReferee})
	}
	// The reporting pass can rewrite words, but cannot change the decision to
	// make a discarded claim true.
	changed := Pos{X: before.X + 1, Y: before.Y}
	wrong, _ := json.Marshal(AutoDecision{Outcomes: out, Units: []AutoUnitEdit{{Unit: enemy.ID, At: &changed}}, Reason: "Repair the prose."})
	if err := r.ApplyReply(mod, run, string(wrong), jsonDecode); err == nil {
		t.Fatal("reporting changed merged state")
	}
	reply, _ := json.Marshal(AutoDecision{Outcomes: out, Reason: "The recorded positions are unchanged."})
	r.FinishRun(run.ID, 0, nil, func(stored *Run) error {
		return r.ApplyReply(mod, stored, string(reply), jsonDecode)
	}, false, time.Now())
	if r.Assistant.Runs[0].Status != RunDone {
		t.Fatal(r.Assistant.Runs[0].Error)
	}
	for _, o := range r.Assistant.Decision.Outcomes {
		if strings.Contains(o.Text, "broke east") {
			t.Fatal("contradictory draft survived regeneration")
		}
	}
	if enemy.Pos() != before || r.Unit(enemy.ID).Pos() != before || r.Phase != PhaseResolving || len(r.Reports) != 0 {
		t.Fatal("report regeneration published or changed the board")
	}
	autoFinish(t, r, mod, "The turn is complete.", false)
	if len(r.Reports) != 1 || len(r.Reports[0].Entries) != len(out) {
		t.Fatal("corrected turn was not published")
	}
	for _, entry := range r.Reports[0].Entries {
		if strings.Contains(entry.Text, "broke east") {
			t.Fatal("contradictory wording reached a published report")
		}
	}
}

func TestConnectedEngagementHasOneWriterAndSharedFinalRecord(t *testing.T) {
	r, mod := batchRoom(t)
	o := r.LockedOrders()[0]
	a, b := r.Unit(o.Unit), r.Unit("z1")
	ct, err := r.AddContest(r.Referee(), mod, o.ID, "Exchange", PartyInput{Unit: a.ID}, &PartyInput{Unit: b.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(r.Referee(), ct.ID, 9, 2, time.Now()); err != nil {
		t.Fatal(err)
	}
	other, err := r.AddContest(r.Referee(), mod, "", "Connected exchange", PartyInput{Unit: b.ID}, &PartyInput{Unit: "load-00"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(r.Referee(), other.ID, 2, 7, time.Now()); err != nil {
		t.Fatal(err)
	}
	r.prepareAutoEngagements()
	p := r.Assistant.Batches
	writer := -1
	for i, group := range p.Groups {
		for _, id := range group {
			if id == a.ID || id == b.ID || id == "load-00" {
				if writer >= 0 && writer != i {
					t.Fatal("connected engagement has multiple writers")
				}
				writer = i
			}
		}
	}
	if writer < 0 || len(p.Engagements) != 2 {
		t.Fatal("missing shared engagements")
	}
	oldA, oldB := a.Strength, b.Strength
	leftA, leftB := oldA-1, oldB-1
	for i := range p.Groups {
		p.Index = i
		d := AutoDecision{Reason: "The shared dice determine both sides."}
		for _, o := range r.autoBatchOrders() {
			d.Outcomes = append(d.Outcomes, AutoOutcome{Order: o.ID, Title: "Exchange", Text: "Both sides took losses and held.", Seen: SeenByReferee})
		}
		if i == writer {
			d.Units = []AutoUnitEdit{{Unit: a.ID, Strength: &leftA}, {Unit: b.ID, Strength: &leftB}}
			desk := r.autoBatchDesk()
			if len(desk.Contests) != 2 || desk.Contests[0].Result.A.Die != 9 || desk.Contests[1].Result.B.Die != 7 {
				t.Fatal("shared dice changed")
			}
		}
		if err := r.validateBatchDecision(mod, &d); err != nil {
			t.Fatal(err)
		}
		p.Decisions = append(p.Decisions, d)
	}
	if err := r.mergeAutoBatches(mod); err != nil {
		t.Fatal(err)
	}
	final := p.Engagements[0].Final
	if len(final) != 2 || final[0].ID != a.ID || final[0].Strength != leftA || final[1].ID != b.ID || final[1].Strength != leftB {
		t.Fatal("both sides missing from shared final record")
	}
	if r.Unit(a.ID).Strength != oldA || r.Unit(b.ID).Strength != oldB {
		t.Fatal("shared proposal changed authoritative units")
	}
	p.Index = writer
	if r.splitFailedAutoBatch() {
		t.Fatal("report repair split an engagement")
	}
}
