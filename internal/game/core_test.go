package game

import (
	"slices"
	"strings"
	"testing"
)

func TestFreeTextOrders(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")

	if _, err := r.SaveOrder(hicks, mod, "m1", "  ", "", true, now); err == nil {
		t.Error("committed an empty order")
	}
	if _, err := r.SaveOrder(hicks, mod, "m2", "Okafor dances.", "", true, now); err != ErrForbidden {
		t.Errorf("Hicks ordered Vasquez's marine: %v", err)
	}
	if _, err := r.SaveOrder(hicks, mod, "m1", "Reyes waves.", "9999", true, now); err == nil {
		t.Error("marked a hex off the map")
	}
	o, err := r.SaveOrder(hicks, mod, "m1", "Reyes takes point to the bridge.", "0605", true, now)
	if err != nil || o.Status != OrderCommitted || o.Mark == nil || *o.Mark != [2]int{5, 4} {
		t.Fatalf("order %+v, %v", o, err)
	}
	// The commander can review and send it back.
	if err := r.ReviewOrder(cmd, o.ID, "return", "", now); err == nil {
		t.Error("sent back without a note")
	}
	if err := r.ReviewOrder(cmd, o.ID, "return", "Wait for Doyle.", now); err != nil || o.Status != OrderDraft {
		t.Fatalf("return: %v, %s", err, o.Status)
	}
	if err := r.ReviewOrder(vasq, o.ID, "approve", "", now); err != ErrForbidden {
		t.Errorf("Vasquez reviewed Hicks's order: %v", err)
	}
	// The blind referee can't see or write Hicks's order; the commander can.
	v := BuildView(r, testScenario(t), mod, ref)
	for _, slot := range v.MyOrders {
		if slot.Unit.ID == "m1" {
			t.Error("the blind referee sees Hicks's marine in their orders list")
		}
	}
	if _, err := r.SaveOrder(ref, mod, "m1", "Referee says hi.", "", true, now); err != ErrForbidden {
		t.Errorf("blind referee overwrote a player's order: %v", err)
	}
	if !slices.ContainsFunc(BuildView(r, testScenario(t), mod, cmd).MyOrders, func(s OrderSlot) bool { return s.Order != nil && s.Order.ID == o.ID }) {
		t.Error("Apone can't see his player's order")
	}
	// Out of contact, Hicks's order is his alone.
	r.Unit("m1").RadioDown = true
	r.RefreshComms(mod)
	if slices.ContainsFunc(BuildView(r, testScenario(t), mod, cmd).MyOrders, func(s OrderSlot) bool { return s.Order != nil && s.Order.ID == o.ID }) {
		t.Error("Apone sees the order of a marine who's out of contact")
	}
	r.Unit("m1").RadioDown = false
	r.RefreshComms(mod)
	// At the lock, the draft lapses; Vasquez's committed order locks.
	r.SaveOrder(vasq, mod, "m2", "Okafor covers the street.", "", true, now)
	r.Tick(mod, r.Deadline)
	if o.Status != OrderLapsed || r.CurrentOrder("m2").Status != OrderLocked || len(r.LockedOrders()) != 1 {
		t.Errorf("after lock: m1 %s, m2 %s", o.Status, r.CurrentOrder("m2").Status)
	}
}

func TestDeskContestsOutcomesAndPublish(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes throws a grenade into the lobby.", "", true, now)
	r.Tick(mod, r.Deadline)
	if r.Phase != PhaseResolving || r.Desk == nil || r.Unwritten() != 1 {
		t.Fatalf("phase %s, desk %v, unwritten %d", r.Phase, r.Desk, r.Unwritten())
	}
	if _, err := r.AddContest(hicks, mod, o.ID, "", PartyInput{Unit: "m1", Skill: "shoot"}, nil, false); err != ErrForbidden {
		t.Errorf("a player added a roll: %v", err)
	}
	// Skill from the unit; the opponent's from theirs; a typed-in value for a door.
	c, err := r.AddContest(ref, mod, o.ID, "", PartyInput{Unit: "m1", Skill: "shoot", Advantage: 20, Why: "surprise"},
		&PartyInput{Unit: "z3", Skill: "sneak"}, false)
	if err != nil || c.A.Value != 55 || c.A.Chance() != 75 || c.B.Value != 70 || c.Title != "Sgt. Reyes: shoot" {
		t.Fatalf("contest %+v, %v", c, err)
	}
	door, _ := r.AddContest(ref, mod, o.ID, "The fire door", PartyInput{Unit: "m1", Skill: "tech"}, &PartyInput{Name: "the door", Value: 50}, true)
	if err := r.RollContest(ref, c.ID, 30, 80, now); err != nil || c.Result.Winner != "a" || !c.Manual {
		t.Fatalf("roll: %v %+v", err, c.Result)
	}
	if err := r.RollContest(ref, door.ID, 0, 0, now); err != nil || door.Result == nil || door.Manual {
		t.Fatalf("server roll: %v", err)
	}
	if err := r.SetOutcome(ref, o.ID, "", "", "The lobby goes quiet. Reyes's ears ring.", SeenByAll); err != nil {
		t.Fatal(err)
	}
	r.SetOutcome(ref, "", "", "Behind the lines", "The runners stir.", SeenByReferee)
	// The referee moves Reyes; players still see the board as at the lock.
	before := HexID(r.Unit("m1").Col, r.Unit("m1").Row)
	hex := "0905"
	if err := r.EditUnit(ref, mod, "m1", UnitEdit{Hex: &hex, Status: ptr("ears ringing")}); err != nil {
		t.Fatal(err)
	}
	find := func(v *View, id string) *UnitView {
		for _, g := range v.Forces {
			for i := range g.Units {
				if g.Units[i].ID == id {
					return &g.Units[i]
				}
			}
		}
		return nil
	}
	if u := find(BuildView(r, testScenario(t), mod, vasq), "m1"); u.Hex != before || u.Status != "" {
		t.Errorf("players saw the move before publishing: %+v", u)
	}
	if u := find(BuildView(r, testScenario(t), mod, ref), "m1"); u.Hex != hex {
		t.Errorf("the referee's board: %+v", u)
	}
	if r.Unwritten() != 0 {
		t.Errorf("unwritten %d", r.Unwritten())
	}
	publishTurn(t, r, mod, ref, now)
	rep := r.Reports[0]
	if len(rep.Entries) != 2 || rep.Entries[0].Text != "The lobby goes quiet. Reyes's ears ring." || len(rep.Entries[0].Rolls) != 1 || len(rep.Entries[0].Secret) != 1 {
		t.Fatalf("report %+v", rep.Entries)
	}
	v := BuildView(r, testScenario(t), mod, vasq)
	if len(v.Reports[0].Entries) != 1 || len(v.Reports[0].Entries[0].Secret) != 0 || strings.Contains(v.Reports[0].Entries[0].Text, "runners") {
		t.Errorf("Vasquez's report: %+v", v.Reports[0].Entries)
	}
	if len(BuildView(r, testScenario(t), mod, ref).Reports[0].Entries) != 2 {
		t.Error("the referee doesn't see their own notes")
	}
	if u := find(BuildView(r, testScenario(t), mod, cmd), "m1"); u.Hex != hex || u.Status != "ears ringing" {
		t.Errorf("after publishing: %+v", u)
	}
	if r.Turn != 2 || r.Phase != PhasePlanning || r.LockedUnits != nil {
		t.Errorf("turn %d %s", r.Turn, r.Phase)
	}
}

func ptr[T any](v T) *T { return &v }

func TestOutOfContactMarinesAndHolderOnlyOutcomes(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	home := HexID(u.Col, u.Row)
	r.RefreshComms(mod) // the store does this after every change: Reyes calls in from home
	r.EditUnit(ref, mod, "m1", UnitEdit{RadioDown: ptr(true)})
	r.RefreshComms(mod)
	u.Col, u.Row = 9, 4
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes searches the basement alone.", "", true, now)
	r.Tick(mod, r.Deadline)
	r.SetOutcome(ref, o.ID, "", "", "Something grabs his ankle.", SeenByHolder)
	publishTurn(t, r, mod, ref, now)
	for _, m := range []*Member{vasq, cmd, hicks} {
		v := BuildView(r, testScenario(t), mod, m)
		seen := len(v.Reports[0].Entries) == 1
		want := m != vasq // the holder, and his commander
		if seen != want {
			t.Errorf("%s sees the basement: %v", m.Name, seen)
		}
	}
	// Others see Reyes where he last called in.
	for _, g := range BuildView(r, testScenario(t), mod, vasq).Forces {
		for _, x := range g.Units {
			if x.ID == "m1" && (x.Hex != home || !x.Stale) {
				t.Errorf("Vasquez sees Reyes at %s (stale %v)", x.Hex, x.Stale)
			}
		}
	}
}

func TestFactsAreCoCreated(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")
	f, err := r.ProposeFact(hicks, mod, "m1", "Reyes carries a flare gun.", now)
	if err != nil || f.Status != FactProposed {
		t.Fatal(err)
	}
	if _, err := r.ProposeFact(hicks, mod, "z1", "The shamblers are slow.", now); err == nil {
		t.Error("a player proposed a fact about an enemy unit")
	}
	if len(r.FactsFor(vasq, "")) != 0 || len(r.FactsFor(hicks, "")) != 1 || len(r.FactsFor(ref, "")) != 1 {
		t.Error("a proposal is visible to someone other than its author and the referee")
	}
	if err := r.DecideFact(hicks, f.ID, "accept", "", "", false, now); err != ErrForbidden {
		t.Errorf("a player accepted a fact: %v", err)
	}
	if err := r.DecideFact(ref, f.ID, "accept", "Reyes carries a flare gun with two flares.", "Two, not more.", false, now); err != nil {
		t.Fatal(err)
	}
	if f.Text != "Reyes carries a flare gun with two flares." || f.Proposed != "Reyes carries a flare gun." || len(r.FactsFor(vasq, "m1")) != 1 {
		t.Errorf("accepted fact %+v", f)
	}
	// A private one stays between proposer and referee.
	p, _ := r.ProposeFact(vasq, mod, "", "Okafor knows the night nurse.", now)
	r.DecideFact(ref, p.ID, "accept", "", "", true, now)
	if slices.ContainsFunc(r.FactsFor(hicks, ""), func(x FactView) bool { return x.ID == p.ID }) {
		t.Error("Hicks reads Vasquez's private fact")
	}
	q, _ := r.ProposeFact(vasq, mod, "", "There's a working ambulance in the lot.", now)
	if err := r.WithdrawFact(hicks, q.ID); err != ErrForbidden {
		t.Errorf("Hicks withdrew Vasquez's fact: %v", err)
	}
	r.DecideFact(ref, q.ID, "reject", "", "Not today.", false, now)
	if q.Status != FactRejected || len(r.acceptedFacts("")) != 2 {
		t.Errorf("rejected fact %+v; accepted %v", q, r.acceptedFacts(""))
	}
	// The referee's own facts are true at once.
	g, _ := r.ProposeFact(ref, mod, "", "The east stairwell is blocked.", now)
	if g.Status != FactAccepted {
		t.Errorf("referee fact %s", g.Status)
	}
}

func TestReopenAndCorrectATurn(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes crosses the bridge.", "", true, now)
	r.Tick(mod, r.Deadline)
	hex := "0705"
	r.EditUnit(ref, mod, "m1", UnitEdit{Hex: &hex})
	r.SetOutcome(ref, o.ID, "", "", "Across, no trouble.", SeenByAll)
	publishTurn(t, r, mod, ref, now)
	r.SaveOrder(hicks, mod, "m1", "Reyes keeps going.", "", true, now)
	r.Post(hicks, "all", "Over the bridge.", now)
	if err := r.ReopenTurn(ref, "Reyes should have met the shamblers", now); err != nil {
		t.Fatal(err)
	}
	if r.Turn != 1 || r.Phase != PhaseResolving || len(r.Reports) != 0 || r.Desk == nil || len(r.Desk.Outcomes) != 1 || len(r.Messages) != 1 {
		t.Fatalf("after reopening: turn %d %s, %d reports", r.Turn, r.Phase, len(r.Reports))
	}
	if HexID(r.Unit("m1").Col, r.Unit("m1").Row) != hex {
		t.Error("the referee's move during adjudication was lost")
	}
	r.SetOutcome(ref, o.ID, "", "", "Halfway over, the shamblers come out of the fog.", SeenByAll)
	publishTurn(t, r, mod, ref, now)
	if r.Reports[0].Corrected == "" || !strings.Contains(r.Reports[0].Entries[0].Text, "fog") {
		t.Errorf("corrected report %+v", r.Reports[0])
	}
}

func TestEveryEventProposesValidEffects(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	r.Threat = MaxThreat
	marines := r.marinePositions(mod)
	for _, def := range mod.Events {
		for range 20 {
			ev, err := r.RollEvent(ref, mod, def.ID)
			if err != nil {
				t.Fatalf("%s: %v", def.ID, err)
			}
			for _, e := range ev.Effects {
				if e.Op == "spawn" {
					if mod.TerrainAt(r.Map.TerrainCode(e.Col, e.Row)).Water() {
						t.Errorf("%s: spawn in water at %s", def.ID, HexID(e.Col, e.Row))
					}
					if _, d := nearest(e.Col, e.Row, marines); d < 1 {
						t.Errorf("%s: spawn on top of marines", def.ID)
					}
				}
			}
			if strings.Contains(ev.Report, "{") {
				t.Errorf("%s: unfilled placeholder %q", def.ID, ev.Report)
			}
			r.Events = nil
		}
	}
}

func TestEndOperationKeepsSurvivors(t *testing.T) {
	r, mod, ref, _, _ := orderRoom(t)
	sc := testScenario(t)
	now := central(t, "2026-09-30 13:00")
	r.EditUnit(ref, mod, "m2", UnitEdit{Strength: ptr(0)})
	r.EditUnit(ref, mod, "m1", UnitEdit{Status: ptr("limping"), Skills: map[string]int{"move": 35}})
	if err := r.EndOperation(ref, sc, mod, now); err != nil {
		t.Fatal(err)
	}
	m1 := r.Unit("m1")
	if m1.Operations != 1 || SkillOf(mod, m1, "move") != 35 || !strings.Contains(m1.Record[0], "limping") {
		t.Errorf("survivor %+v", m1)
	}
	if !strings.Contains(r.Unit("m2").Name, "replacements") || r.Operations[0].Lost[0] != "LCpl. Okafor" {
		t.Errorf("the lost: %+v %+v", r.Unit("m2"), r.Operations[0])
	}
}
