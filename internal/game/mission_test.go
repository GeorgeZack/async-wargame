package game

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func missionRoom(t *testing.T) (*Room, *Module, *Scenario) {
	t.Helper()
	cat := testCatalog(t)
	sc := cat.Scenarios["crossroads"]
	mod := cat.Modules[sc.Module]
	r := newRoom("TEST01", "Crossroads", sc, mod, "Ref", "hash")
	if err := r.StartOperation(r.Referee(), mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	return r, mod, sc
}
func finishMissionTurn(t *testing.T, r *Room, mod *Module) {
	t.Helper()
	if err := r.LockNow(r.Referee(), mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.PublishTurn(r.Referee(), mod, time.Now()); err != nil {
		t.Fatal(err)
	}
}
func TestMissionCompletesFromPositionsAndRestoresOnCorrection(t *testing.T) {
	r, mod, sc := missionRoom(t)
	for i := 0; i < 4; i++ {
		finishMissionTurn(t, r, mod)
	}
	if r.Mission.Objectives[0].Status != "complete" || r.MissionComplete() {
		t.Fatal("hold checkpoint failed")
	}
	var at Pos
	for _, f := range r.Features {
		if f.ID == "west" {
			at = f.Pos()
		}
	}
	r.LockNow(r.Referee(), mod, time.Now())
	r.MoveUnits(r.Referee(), mod, []UnitMove{{ID: "line", At: at}, {ID: "scouts", At: at}})
	if r.Mission.Objectives[1].Status != "pending" {
		t.Fatal("objective changed before publication")
	}
	r.PublishTurn(r.Referee(), mod, time.Now())
	if r.Phase != PhaseComplete || r.Turn != 5 || r.Mission.Status != "success" || !r.Deadline.IsZero() || r.Due(time.Now().Add(24*time.Hour)) {
		t.Fatalf("mission didn't stop: %+v", r.Mission)
	}
	if _, err := r.SaveOrder(r.Referee(), mod, "line", "Continue", nil, true, time.Now()); err == nil {
		t.Fatal("finished mission accepted orders")
	}
	if err := r.SetAutomatic(r.Referee(), "enable", time.Now()); err == nil {
		t.Fatal("finished mission queued another judge")
	}
	if err := r.EditUnit(r.Referee(), mod, "line", UnitEdit{Strength: ptr(0)}); err == nil {
		t.Fatal("completed mission changed without correction")
	}
	if err := r.PlanUnits(r.Referee(), mod, []UnitMove{{ID: "line", At: Pos{0, 0}}}, time.Now()); err == nil {
		t.Fatal("completed mission accepted a staged move")
	}
	if !r.CanCorrect() || r.NewTurnOrders() != 0 {
		t.Fatal("final turn can't be corrected")
	}
	raw, _ := json.Marshal(r)
	var restored Room
	json.Unmarshal(raw, &restored)
	r = &restored
	if err := r.ReopenTurn(r.Referee(), "Scouts did not reach the road", time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.Phase != PhaseResolving || r.MissionComplete() || r.Mission.Objectives[1].Status != "pending" {
		t.Fatal("mission result survived rollback")
	}
	r.PublishTurn(r.Referee(), mod, time.Now())
	if r.Mission.Status != "success" || len(r.Reports) != 5 {
		t.Fatal("corrected result duplicated report")
	}
	if err := r.EndOperation(r.Referee(), sc, mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.MissionComplete() || r.Mission.Objectives[0].Status != "pending" || r.Operations[0].Result != "success" {
		t.Fatal("new operation lost result or kept objective progress")
	}
}
func TestMissionFailureAndHoldCannotBeRecoveredByProse(t *testing.T) {
	r, mod, _ := missionRoom(t)
	// One missed hold turn permanently fails that condition, even if the unit returns.
	r.Unit("line").X = 0
	r.Unit("scouts").X = 0
	finishMissionTurn(t, r, mod)
	if r.Mission.Objectives[0].Status != "failed" {
		t.Fatal("missed hold succeeded")
	}
	for r.Phase != PhaseComplete {
		finishMissionTurn(t, r, mod)
	}
	if r.Turn != 6 || r.Mission.Status != "failure" {
		t.Fatal("deadline did not produce failure")
	}
	r, mod, _ = missionRoom(t)
	r.Unit("line").Strength = 0
	r.Unit("scouts").Strength = 0
	finishMissionTurn(t, r, mod)
	if r.Turn != 1 || r.Mission.Status != "failure" {
		t.Fatal("annihilation did not end mission")
	}
}
func TestMissionContestedAreaAndValidation(t *testing.T) {
	r, mod, sc := missionRoom(t)
	p := r.Unit("line").Pos()
	r.Unit("east").X, r.Unit("east").Y = p.X, p.Y
	finishMissionTurn(t, r, mod)
	if r.Mission.Objectives[0].Status != "failed" {
		t.Fatal("contested area counted as held")
	}
	copy := *sc
	copy.Mission = append([]MissionRule{}, sc.Mission...)
	for _, mutate := range []func(*MissionRule){func(x *MissionRule) { x.Units = []string{"east"} }, func(x *MissionRule) { x.Feature = "unknown" }, func(x *MissionRule) { x.Through = 0 }, func(x *MissionRule) { x.Kind = "invent" }} {
		copy.Mission[0] = sc.Mission[0]
		mutate(&copy.Mission[0])
		if err := copy.validate(mod); err == nil {
			t.Fatal("invalid mission rule accepted")
		}
	}
}
func TestAutomaticMissionNarrativeUsesServerResult(t *testing.T) {
	r, mod, _ := missionRoom(t)
	r.Unit("line").Strength = 0
	r.Unit("scouts").Strength = 0
	r.SetAutomatic(r.Referee(), "enable", time.Now())
	r.LockNow(r.Referee(), mod, time.Now())
	autoToResolution(t, r, mod)
	autoFinish(t, r, mod, autoDecisionReply(r), false)
	run := r.NextRun("test", "mock", time.Now())
	_, user, ok := r.PromptFor(mod, run)
	if !ok || !strings.Contains(user, "Mission failure") || r.MissionComplete() {
		t.Fatal("narrator missing result or preview leaked into live state")
	}
	r.FinishRun(run.ID, 10, nil, func(stored *Run) error {
		return r.ApplyReply(mod, stored, "Both detachments were lost. The mission failed.", jsonDecode)
	}, false, time.Now())
	if r.Phase != PhaseComplete || r.Mission.Status != "failure" {
		t.Fatal("automatic final turn didn't complete")
	}
	if err := r.ReopenTurn(r.Referee(), "Restore a survivor", time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.MissionComplete() || r.Assistant.Automatic {
		t.Fatal("automatic mission correction failed")
	}
}
