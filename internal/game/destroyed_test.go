package game

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDestroyedUnitsLeaveFormationAndCannotOrder(t *testing.T) {
	r, mod, ref, cmd, player := orderRoom(t)
	for _, u := range r.livingMembers("lantern") {
		u.AssignedTo = player.ID
	}
	now := time.Now()
	if _, err := r.SaveOrder(cmd, mod, "f-lantern", "Advance together", nil, true, now); err != nil {
		t.Fatal(err)
	}
	corpse := r.Unit("m1")
	original := corpse.Pos()
	if err := r.EditUnit(ref, mod, corpse.ID, UnitEdit{Strength: ptr(0)}); err != nil {
		t.Fatal(err)
	}
	if r.CanOrder(mod, player, corpse) || r.CanOrder(mod, ref, corpse) {
		t.Fatal("destroyed unit can take orders")
	}
	if _, err := r.SaveOrder(player, mod, corpse.ID, "Keep walking", nil, true, now); err == nil {
		t.Fatal("dead unit accepted order")
	}
	if err := r.SplitOff(ref, corpse.ID); err == nil {
		t.Fatal("dead unit can split off")
	}
	if _, total, _ := r.OrderReadiness(); total != 3 {
		t.Fatalf("readiness includes casualty: %d", total)
	}
	at := Pos{original.X + 40, original.Y}
	if err := r.MoveUnits(ref, mod, []UnitMove{{ID: "m2", At: at}}); err != nil {
		t.Fatal(err)
	}
	if corpse.Pos() != original {
		t.Fatal("corpse followed formation")
	}
	v := BuildView(r, testScenario(t), mod, player)
	deadToken := false
	deadPip := false
	for _, s := range v.Map.Stacks {
		if s.Top.ID == corpse.ID {
			deadToken = s.Top.Destroyed && s.Chit && strings.Contains(s.Label, "destroyed")
		}
		if s.Top.ID == "f-lantern" {
			if len(s.Top.Members) != 3 {
				t.Fatal("dead member grouped into formation")
			}
			for _, p := range s.Pips {
				deadPip = deadPip || p.Destroyed
			}
		}
	}
	if !deadToken || !deadPip {
		t.Fatal("casualty token/pip missing")
	}
	if _, err := r.SaveOrder(cmd, mod, "f-lantern", "Survivors hold", nil, true, now); err != nil {
		t.Fatal(err)
	}
	r.LockNow(ref, mod, now)
	if r.CurrentOrder(corpse.ID).Status != OrderLapsed {
		t.Fatal("casualty order was locked")
	}
	if len(r.LockedOrders()) != 1 || len(r.OrderMembers(r.LockedOrders()[0])) != 3 {
		t.Fatal("surviving squad lost its global order")
	}
	if err := r.EditUnit(ref, mod, corpse.ID, UnitEdit{Strength: ptr(1)}); err != nil {
		t.Fatal(err)
	}
	if !corpse.Detached || corpse.Pos() != original {
		t.Fatal("restoring a distant casualty merged incompatible positions")
	}
}

func TestDestroyedStatePublishesWithTurnAndRespectsStaleContact(t *testing.T) {
	r, mod, ref, cmd, player := orderRoom(t)
	u := r.Unit("m1")
	r.LockNow(ref, mod, time.Now())
	r.EditUnit(ref, mod, u.ID, UnitEdit{Strength: ptr(0)})
	v := BuildView(r, testScenario(t), mod, player)
	if slices.ContainsFunc(v.Data.Units, func(x UnitView) bool { return x.ID == u.ID && x.Destroyed }) {
		t.Fatal("casualty leaked before report")
	}
	r.PublishTurn(ref, mod, time.Now())
	v = BuildView(r, testScenario(t), mod, player)
	if !slices.ContainsFunc(v.Data.Units, func(x UnitView) bool { return x.ID == u.ID && x.Destroyed }) {
		t.Fatal("published casualty absent")
	}
	old := u.Pos()
	u.Reported = &old
	u.OutOfContact = true
	uv := unitView(r, u, mod, cmd)
	if uv.Destroyed || uv.Strength != "Last contact" || uv.Status != "" {
		t.Fatal("stale contact leaks casualty")
	}
	if !unitView(r, u, mod, player).Destroyed {
		t.Fatal("own casualty hidden")
	}
}

func TestAutomaticRefereeRejectsDestroyedContestAndMovement(t *testing.T) {
	r, mod, ref := autoRoom(t)
	r.Unit("z1").Strength = 0
	autoFinish(t, r, mod, autoIntentReply(r, mod), false)
	run := autoFinish(t, r, mod, `{"contests":[{"title":"Dead attack","a":{"unit":"z1"}}]}`, true)
	if !strings.Contains(run.Error, "destroyed") {
		t.Fatal(run.Error)
	}
	if err := r.SetAutomatic(ref, "retry", time.Now()); err != nil {
		t.Fatal(err)
	}
	retry := r.NextRun("test", "mock", time.Now())
	system, _, _ := r.PromptFor(mod, retry)
	if !strings.Contains(system, run.Error) {
		t.Fatal("retry didn't receive validation feedback")
	}
	// An already destroyed counter must not move through an AI decision.
	d := AutoDecision{Reason: "Invalid movement"}
	for _, o := range r.LockedOrders() {
		d.Outcomes = append(d.Outcomes, AutoOutcome{Order: o.ID, Title: "Hold", Text: "Held", Seen: SeenByAll})
	}
	p := r.Unit("z1").Pos()
	p.X++
	d.Units = []AutoUnitEdit{{Unit: "z1", At: &p}}
	if err := r.applyAutoDecision(mod, &d); err == nil || !strings.Contains(err.Error(), "destroyed") {
		t.Fatal(err)
	}
}

func TestCasualtyCanRemainWhileSurvivingFormationMoves(t *testing.T) {
	r, mod, _ := autoRoom(t)
	autoToResolution(t, r, mod)
	var d AutoDecision
	jsonDecode(autoDecisionReply(r), &d)
	at := r.Unit("m1").Pos()
	at.X++
	d.Units = []AutoUnitEdit{{Unit: "m1", Strength: ptr(0)}, {Unit: "m2", At: &at}, {Unit: "m3", At: &at}, {Unit: "m4", At: &at}}
	before := r.Unit("m1").Pos()
	if err := r.applyAutoDecision(mod, &d); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != before || r.Unit("m2").Pos() != at {
		t.Fatal("casualty moved with surviving formation")
	}
}
