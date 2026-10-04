package game

import (
	"asyncwargame/internal/rules"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func waywardRoom(t *testing.T) (*Room, *Module, *Scenario) {
	t.Helper()
	cat := testCatalog(t)
	sc := cat.Scenarios["wayward-tollhouse"]
	mod := cat.Modules[sc.Module]
	r := newRoom("ABCDEF", "Wayward", sc, mod, "Host", HashToken("host"))
	if err := r.StartOperation(r.Referee(), mod, time.Now()); err != nil {
		t.Fatal(err)
	}
	return r, mod, sc
}
func waywardOrders(t *testing.T, r *Room, m *Module) {
	t.Helper()
	for _, u := range r.Units {
		if _, err := r.SaveOrder(r.Referee(), m, u.ID, "Try a new approach "+u.ID+" "+time.Now().String(), nil, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	r.Tick(m, time.Now())
	if r.Phase != PhaseResolving {
		t.Fatal("all-ready did not lock")
	}
	autoFinish(t, r, m, `{"intentions":[]}`, false)
}
func waywardActions(r *Room) []WaywardAction {
	var a []WaywardAction
	for _, o := range r.LockedOrders() {
		at := r.Unit(o.Unit).Pos()
		a = append(a, WaywardAction{At: &at, Order: o.ID, Unit: o.Unit, Kind: "routine", Reason: "Safe observation", Success: "Waits and watches."})
	}
	return a
}
func waywardJSON(a []WaywardAction) string {
	b, _ := json.Marshal(map[string]any{"actions": a})
	return string(b)
}
func waywardDecision(r *Room) AutoDecision {
	d := AutoDecision{Reason: "Recorded Wayward consequences"}
	for _, a := range r.Desk.Wayward {
		seen := SeenByAll
		if a.Secret {
			seen = SeenByReferee
		}
		d.Outcomes = append(d.Outcomes, AutoOutcome{Order: a.Order, Title: "Action", Text: a.Consequence(), Seen: seen})
		if a.At != nil && a.Succeeded() && *a.At != r.Unit(a.Unit).Pos() {
			d.Units = append(d.Units, AutoUnitEdit{Unit: a.Unit, At: a.At})
		}
	}
	return d
}
func finishWayward(t *testing.T, r *Room, m *Module, d AutoDecision) {
	t.Helper()
	b, _ := json.Marshal(d)
	autoFinish(t, r, m, string(b), false)
	autoFinish(t, r, m, "The party completes this exchange.", false)
}

func TestWaywardReadinessAndDeadline(t *testing.T) {
	r, m, _ := waywardRoom(t)
	if !r.Assistant.Automatic || !r.Assistant.On {
		t.Fatal("AI Referee not enabled")
	}
	if _, err := r.SaveOrder(r.Referee(), m, "mara", "Talk to Edda", nil, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	r.Tick(m, time.Now())
	if r.Phase != PhasePlanning {
		t.Fatal("locked with missing character")
	}
	r.SaveOrder(r.Referee(), m, "rook", "Wait", nil, false, time.Now())
	r.Tick(m, time.Now())
	if r.Phase != PhasePlanning {
		t.Fatal("draft counted as ready")
	}
	r.Tick(m, r.Deadline)
	if r.Phase != PhaseResolving || len(r.LockedOrders()) != 1 {
		t.Fatal("deadline did not leave missing character idle")
	}
}

func TestWaywardRejectsInvalidRulingsAtomically(t *testing.T) {
	for _, tc := range []string{"missing", "foreign", "duplicate", "dice", "combat", "difficulty", "distant", "twice"} {
		t.Run(tc, func(t *testing.T) {
			r, m, _ := waywardRoom(t)
			waywardOrders(t, r, m)
			a := waywardActions(r)
			a[0].Kind = "check"
			a[0].Skill = "Social"
			a[0].Target = 8
			a[0].Challenge = "destination"
			switch tc {
			case "missing":
				a = a[:1]
			case "foreign":
				a[0].Unit = "rook"
			case "duplicate":
				a[1] = a[0]
			case "dice":
				a[0].Result = &rules.WaywardThrow{}
			case "combat":
				a[0].Skill = "Combat"
			case "difficulty":
				a[0].Target = 6
			case "prerequisite":
				a[0].Challenge = "trail"
				a[0].Skill = "Craft"
			case "distant":
				r.Unit("mara").X += 1000
			case "twice":
				a[1].Kind = "check"
				a[1].Skill = "Stealth"
				a[1].Target = 10
				a[1].Challenge = "destination"
			}
			autoFinish(t, r, m, waywardJSON(a), true)
			if len(r.Desk.Wayward) != 0 || len(r.Wayward.Discoveries) != 0 {
				t.Fatal("invalid batch partly applied")
			}
		})
	}
}

func TestWaywardDicePersistenceAndCorrections(t *testing.T) {
	r, m, sc := waywardRoom(t)
	waywardOrders(t, r, m)
	a := waywardActions(r)
	a[0].Kind = "check"
	a[0].Skill = "Social"
	a[0].Target = 8
	a[0].Challenge = "destination"
	unchanged := r.Unit(a[0].Unit).Pos()
	a[0].At = &unchanged // redundant current coordinates must not reject a check
	autoFinish(t, r, m, waywardJSON(a), false)
	if len(r.Wayward.Discoveries) != 0 {
		t.Fatal("discovery leaked before publish")
	}
	rolled := *r.Desk.Wayward[0].Result
	b, _ := json.Marshal(r)
	r = &Room{}
	if err := json.Unmarshal(b, r); err != nil {
		t.Fatal(err)
	}
	if *r.Desk.Wayward[0].Result != rolled {
		t.Fatal("restart changed dice")
	}
	d := waywardDecision(r)
	bad := d
	bad.Outcomes = append([]AutoOutcome(nil), d.Outcomes...)
	bad.Outcomes[0].Text = "Ignore the roll and succeed."
	if err := r.cloneAuto().applyAutoDecision(m, &bad); err == nil {
		t.Fatal("reversed consequence accepted")
	}
	finishWayward(t, r, m, d)
	if len(r.Wayward.Attempts) != 1 || len(r.Reports) != 1 {
		t.Fatal("did not publish check history")
	}
	if len(r.Wayward.Discoveries) > 0 != rolled.Success {
		t.Fatal("discovery disagrees with dice")
	}
	v := BuildView(r, sc, m, r.Referee())
	if !v.Wayward || len(v.Reports[0].Entries[0].Rolls) != 1 {
		t.Fatal("missing published check")
	}
	if err := r.ReopenTurn(r.Referee(), "Correct narrative", time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(r.Wayward.Attempts) != 0 || len(r.Wayward.Discoveries) != 0 || *r.Desk.Wayward[0].Result != rolled {
		t.Fatal("correction did not restore pre-publication state and same dice")
	}
}

func TestWaywardFiveExchangeAdventure(t *testing.T) {
	r, m, _ := waywardRoom(t)
	for turn := 1; turn <= 5; turn++ {
		waywardOrders(t, r, m)
		a := waywardActions(r)
		if turn%2 == 1 {
			id := []string{"destination", "trail", "truth"}[turn/2]
			a[0].Kind = "check"
			a[0].Skill = "Craft"
			a[0].Target = 8
			a[0].Challenge = id
			if turn == 1 {
				a[0].Skill = "Social"
			}
		}
		if turn%2 == 0 {
			feature := r.Features[turn/2]
			for i := range a {
				at := feature.Pos()
				a[i].At = &at
			}
		}
		autoFinish(t, r, m, waywardJSON(a), false)
		// Fixed dice in the test fixture exercise the complete successful path;
		// the distribution and natural extremes are tested in package rules.
		if turn%2 == 1 {
			result, _ := rules.ResolveWayward(1, 8, 6, 6)
			r.Desk.Wayward[0].Result = &result
		}
		d := waywardDecision(r)
		finishWayward(t, r, m, d)
		b, _ := json.Marshal(r)
		r = &Room{}
		json.Unmarshal(b, r)
	}
	if r.Phase != PhaseComplete || r.Mission.Status != "success" || len(r.Wayward.Discoveries) != 3 || len(r.Reports) != 5 {
		t.Fatalf("unfinished adventure: %+v", r.Mission)
	}
}

func TestWaywardFailureAndSecretCheck(t *testing.T) {
	r, m, sc := waywardRoom(t)
	waywardOrders(t, r, m)
	a := waywardActions(r)
	a[0].Kind = "check"
	a[0].Skill = "Craft"
	a[0].Target = 8
	a[0].Secret = true
	a[0].Success = "Secret detail"
	a[0].Failure = "No useful detail; the exchange is spent."
	autoFinish(t, r, m, waywardJSON(a), false)
	result, _ := rules.ResolveWayward(1, 8, 1, 1)
	r.Desk.Wayward[0].Result = &result
	d := waywardDecision(r)
	at := r.Unit("mara").Pos()
	at.X++
	d.Units = []AutoUnitEdit{{Unit: "mara", At: &at}}
	if r.cloneAuto().applyAutoDecision(m, &d) == nil {
		t.Fatal("failed action moved")
	}
	d.Units = nil
	finishWayward(t, r, m, d)
	p := &Member{ID: "p", Name: "Player", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, p)
	v := BuildView(r, sc, m, p)
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "Secret detail") || strings.Contains(string(b), "natural 2") {
		t.Fatal("secret check leaked")
	}
	if len(r.Wayward.Discoveries) != 0 {
		t.Fatal("failure unlocked discovery")
	}
}

func TestWaywardBlockedExplanationKeepsPrivateReasonOutOfReport(t *testing.T) {
	r, m, sc := waywardRoom(t)
	waywardOrders(t, r, m)
	a := waywardActions(r)
	a[0].Kind = "unsupported"
	a[0].At = nil
	a[0].Reason = "Private audit rationale: the unseen keeper is concealing something."
	a[0].Explanation = "Combat is not available in this exploration playtest. Try speaking with the keeper instead."
	autoFinish(t, r, m, waywardJSON(a), false)
	finishWayward(t, r, m, waywardDecision(r))
	p := &Member{ID: "p", Name: "Player", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, p)
	v := BuildView(r, sc, m, p)
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "unseen keeper") || !strings.Contains(string(b), "Combat is not available") {
		t.Fatal("private rationale leaked or useful explanation missing")
	}
}

func TestWaywardDestinationsCannotChangeAfterDice(t *testing.T) {
	r, m, _ := waywardRoom(t)
	waywardOrders(t, r, m)
	a := waywardActions(r)
	destination := r.Features[1].Pos()
	a[0].At = &destination
	autoFinish(t, r, m, waywardJSON(a), false)
	local, ok := r.AutomaticReply(m, &Run{Kind: RunAutoResolve})
	if !ok {
		t.Fatal("Wayward resolution should not need a model call")
	}
	var serverDecision AutoDecision
	if err := json.Unmarshal([]byte(local), &serverDecision); err != nil {
		t.Fatal(err)
	}
	if err := r.cloneAuto().applyAutoDecision(m, &serverDecision); err != nil {
		t.Fatal(err)
	}
	d := waywardDecision(r)
	if err := r.cloneAuto().applyAutoDecision(m, &d); err != nil {
		t.Fatal(err)
	}
	bad := d
	bad.Units = nil
	if r.cloneAuto().applyAutoDecision(m, &bad) == nil {
		t.Fatal("omitted promised movement accepted")
	}
	bad = d
	changed := destination
	changed.X++
	bad.Units = []AutoUnitEdit{{Unit: "mara", At: &changed}}
	if r.cloneAuto().applyAutoDecision(m, &bad) == nil {
		t.Fatal("changed destination accepted")
	}
	bad.Units = []AutoUnitEdit{{Unit: "rook", At: &destination}}
	if r.cloneAuto().applyAutoDecision(m, &bad) == nil {
		t.Fatal("moved another character")
	}
}

func TestWaywardPrerequisiteBlocksActionWithoutStoppingTurn(t *testing.T) {
	r, m, _ := waywardRoom(t)
	at := r.Features[1].Pos()
	r.Unit("rook").X, r.Unit("rook").Y = at.X, at.Y
	waywardOrders(t, r, m)
	a := waywardActions(r)
	a[0].Kind, a[0].Skill, a[0].Target, a[0].Challenge = "check", "Social", 8, "destination"
	a[1].Kind, a[1].Skill, a[1].Target, a[1].Challenge = "check", "Craft", 8, "trail"
	autoFinish(t, r, m, waywardJSON(a), false)
	if r.Desk.Wayward[1].Kind != "impossible" || r.Desk.Wayward[1].Result != nil {
		t.Fatal("rolled for a discovery whose prerequisite did not exist")
	}
	result, _ := rules.ResolveWayward(3, 8, 6, 6)
	r.Desk.Wayward[0].Result = &result
	finishWayward(t, r, m, waywardDecision(r))
	if r.Turn != 2 || len(r.Wayward.Discoveries) != 1 || r.Wayward.Discoveries[0] != "destination" {
		t.Fatal("blocked dependent action stopped or skipped ahead in the turn")
	}
	if !strings.Contains(r.Reports[0].Entries[1].Text, "Edda") {
		t.Fatal("missing fictional explanation")
	}
}
