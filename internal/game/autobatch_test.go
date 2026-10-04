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
	r.queueRun(RunAutoResolve, "batch-2", r.Turn, time.Now())
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
