package game

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAutomaticContestRejectsDuplicateAndUnrelatedExchanges(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{"same pair", "m1", "z1"},
		{"reversed pair", "z1", "m1"},
		{"self opposition", "m1", "m1"},
		{"unrelated order", "m2", "z2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mod, ref := autoRoom(t)
			autoFinish(t, r, mod, autoIntentReply(r, mod), false)
			o := r.LockedOrders()[0]
			ct, err := r.AddContest(ref, mod, o.ID, "original", PartyInput{Unit: "m1"}, &PartyInput{Unit: "z1"}, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.RollContest(ref, ct.ID, 2, 8, time.Now()); err != nil {
				t.Fatal(err)
			}
			before := ct.Result.Text()
			reply, _ := json.Marshal(map[string]any{"contests": []any{map[string]any{"order": o.ID, "title": "Different title", "a": map[string]string{"unit": tc.a}, "b": map[string]string{"unit": tc.b}}}})
			autoFinish(t, r, mod, string(reply), true)
			if len(r.Desk.Contests) != 1 || r.Desk.Contests[0].Result.Text() != before {
				t.Fatal("rejected contest changed committed dice")
			}
		})
	}
}

func TestContestPlanningIsBlindToRecordedDice(t *testing.T) {
	r, mod := batchRoom(t)
	owner := r.Assistant.Batches.Groups[0][0]
	ct, err := r.AddContest(r.Referee(), mod, "", "existing exchange", PartyInput{Unit: owner}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(r.Referee(), ct.ID, 10, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	r.Assistant.Batches.Input = r.autoFingerprint()
	run := &Run{Kind: RunAutoContests, Turn: r.Turn, Operation: r.Operation}
	_, body, ok := r.autoPrompt(mod, run)
	if !ok {
		t.Fatal("missing prompt")
	}
	if !strings.Contains(body, "existing exchange") || strings.Contains(body, `"result"`) || strings.Contains(body, `"die"`) || strings.Contains(body, `"rolledAt"`) {
		t.Fatal("contest planning lost definitions or saw dice", body)
	}
	run.Kind = RunAutoResolve
	_, body, ok = r.autoPrompt(mod, run)
	if !ok || !strings.Contains(body, `"die":10`) || ct.Result.A.Die != 10 {
		t.Fatal("resolution lost original dice")
	}
}

func TestUnidentifiedContactDoesNotRevealIdentityThroughDice(t *testing.T) {
	r, mod, _ := autoRoom(t)
	r.Unit("z1").Known, r.Unit("z1").Identified = true, false
	autoFinish(t, r, mod, autoIntentReply(r, mod), false)
	autoFinish(t, r, mod, `{"contests":[{"order":"","title":"Contact fires","a":{"unit":"m1"},"b":{"unit":"z1"},"secret":false}]}`, false)
	if !r.Desk.Contests[0].Secret {
		t.Fatal("anonymous contact's true identity and quality leaked through dice")
	}
}

func TestOrdersCloseAtDeadlineBeforeClockTick(t *testing.T) {
	now := time.Now()
	r, mod, ref := startedRoom(t, now)
	r.Deadline = now.Add(time.Minute)
	o, err := r.SaveOrder(ref, mod, "m1", "Hold", nil, true, r.Deadline.Add(-time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{r.Deadline, r.Deadline.Add(time.Second)} {
		before, _ := json.Marshal(r)
		if _, err := r.SaveOrder(ref, mod, "m1", "Late change", nil, true, at); err == nil {
			t.Fatal("late order accepted")
		}
		if _, err := r.SaveSquadOrder(ref, mod, "lantern", "Late squad order", nil, true, at); err == nil {
			t.Fatal("late squad order accepted")
		}
		if err := r.WithdrawOrder(ref, mod, o.ID, at); err == nil {
			t.Fatal("late withdrawal accepted")
		}
		if err := r.ReviewOrder(ref, o.ID, "return", "Late return", at); err == nil {
			t.Fatal("late review accepted")
		}
		after, _ := json.Marshal(r)
		if string(before) != string(after) {
			t.Fatal("rejected late action changed state")
		}
	}
	r.Tick(mod, r.Deadline)
	if r.Phase != PhaseResolving || r.LockedOrders()[0].Text != "Hold" {
		t.Fatal("deadline changed accepted order")
	}
}

func TestBothOrdersReportSharedExchangeDice(t *testing.T) {
	r, mod, ref := startedRoom(t, time.Now())
	for _, id := range []string{"m1", "m2"} {
		if _, err := r.SaveOrder(ref, mod, id, "Contest the crossing", nil, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.LockNow(ref, mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	ct, err := r.AddContest(ref, mod, r.CurrentOrder("m1").ID, "Shared exchange", PartyInput{Unit: "m1"}, &PartyInput{Unit: "m2"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(ref, ct.ID, 3, 7, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.PublishTurn(ref, mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	entries := r.Reports[0].Entries
	if len(entries) != 2 || len(entries[0].Rolls) != 1 || len(entries[1].Rolls) != 1 || entries[0].Rolls[0] != entries[1].Rolls[0] {
		t.Fatal("one side could not audit the shared roll", entries)
	}
}

func TestPlayerOrderPanelsStayFrozenDuringAdjudication(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	if _, err := r.SaveOrder(player, mod, "m1", "Hold", nil, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.LockNow(ref, mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	before := BuildView(r, testScenario(t), mod, player)
	if len(before.MyOrders) != 1 {
		t.Fatal("fixture missing order")
	}
	u := r.Unit("m1")
	u.Strength, u.Name, u.Status, u.X = 0, "UNPUBLISHED-NAME", "UNPUBLISHED-STATUS", u.X+10
	u.RadioDown = true
	r.RefreshComms(mod)
	after := BuildView(r, testScenario(t), mod, player)
	if len(after.MyOrders) != 1 || !reflect.DeepEqual(after.MyOrders[0].Unit, before.MyOrders[0].Unit) {
		t.Fatal("order panel exposed intermediate casualty or position")
	}
	data, _ := json.Marshal(after)
	if strings.Contains(string(data), "UNPUBLISHED-") {
		t.Fatal("unpublished unit state leaked into player view")
	}
	card, err := BuildUnitCard(r, mod, player, "m1")
	if err != nil || card.Unit.Name == "UNPUBLISHED-NAME" || card.Unit.Destroyed {
		t.Fatal("card exposed unpublished state")
	}
	if !BuildView(r, testScenario(t), mod, ref).Assignable[0].Destroyed {
		t.Fatal("referee lost current state")
	}
}

func TestManualDiceOverridesLeavePrivateAuditRecord(t *testing.T) {
	r, mod, ref := autoRoom(t)
	ct, err := r.AddContest(ref, mod, "", "Audit exchange", PartyInput{Unit: "m1"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(ref, ct.ID, 1, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.RollContest(ref, ct.ID, 10, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteContest(ref, ct.ID); err != nil {
		t.Fatal(err)
	}
	entries := r.Log[len(r.Log)-2:]
	if entries[0].Visibility != VisReferee || entries[1].Visibility != VisReferee || !strings.Contains(entries[0].Text, "rolled 1,") || !strings.Contains(entries[0].Text, "rolled 10,") || !strings.Contains(entries[1].Text, "deleted rolled contest") {
		t.Fatal("missing private override history", entries)
	}
}

func TestUnidentifiedNamesCannotPublishInOutcomesOrBriefings(t *testing.T) {
	r, mod, ref := autoRoom(t)
	u := r.Unit("z1")
	u.Name, u.Known, u.Identified = "Wood Raiders", true, false
	autoToResolution(t, r, mod)
	var d AutoDecision
	if err := json.Unmarshal([]byte(autoDecisionReply(r)), &d); err != nil {
		t.Fatal(err)
	}
	d.Outcomes[0].Text = "WOOD RAIDERS fired from the wood."
	reply, _ := json.Marshal(d)
	autoFinish(t, r, mod, string(reply), true)
	if len(r.Reports) != 0 || r.Assistant.Decision != nil {
		t.Fatal("secret name was accepted")
	}
	if err := r.SetAutomatic(ref, "retry", time.Now()); err != nil {
		t.Fatal(err)
	}
	d.Outcomes[0].Text = "An unidentified contact fired from the wood."
	reply, _ = json.Marshal(d)
	autoFinish(t, r, mod, string(reply), false)
	autoFinish(t, r, mod, "Wood Raiders opened fire.", true)
	if len(r.Reports) != 0 || r.Phase != PhaseResolving {
		t.Fatal("briefing leaked the true identity")
	}
	if err := r.SetAutomatic(ref, "retry", time.Now()); err != nil {
		t.Fatal(err)
	}
	autoFinish(t, r, mod, "An unidentified contact opened fire.", false)
	if len(r.Reports) != 1 {
		t.Fatal("safe report did not publish")
	}
}
