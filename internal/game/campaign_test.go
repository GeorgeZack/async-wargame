package game

import (
	"strings"
	"testing"
)

func TestEndOperationCarriesSurvivors(t *testing.T) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	sc := testScenario(t)
	r.Unit("m1").Operations = 1 // already Seasoned
	r.Unit("m1").Col, r.Unit("m1").Row, r.Unit("m1").AssignedTo = 6, 6, "someone"
	r.Units = deleteUnit(r.Units, "m2") // the 2nd Rifle Squad was lost
	r.Unit("z1").Col = 20               // the infected moved

	if err := r.EndOperation(ref, sc, mod, central(t, "2026-10-02 12:00")); err != nil {
		t.Fatal(err)
	}
	m1 := r.Unit("m1")
	if m1.Operations != 2 || ExperienceOf(mod, m1).Name != "Veteran" || !strings.Contains(m1.Record[0], "Promoted to Veteran") {
		t.Errorf("survivor: %d ops, %s, %v", m1.Operations, ExperienceOf(mod, m1).Name, m1.Record)
	}
	if m1.Col != 5 || m1.Row != 7 || m1.AssignedTo != "someone" {
		t.Error("survivor didn't redeploy at its start hex with its commander")
	}
	m2 := r.Unit("m2")
	if m2 == nil || m2.Operations != 0 || !strings.Contains(m2.Name, "replacements") {
		t.Errorf("no green replacement for the lost squad: %+v", m2)
	}
	if r.Unit("z1").Col != 13 {
		t.Error("the infected didn't reset to the scenario")
	}
	if r.Operation != 2 || r.Phase != PhaseSetup || r.Turn != 0 || len(r.Operations) != 1 || r.Operations[0].Lost[0] != "2nd Rifle Squad" {
		t.Errorf("campaign state: op %d phase %s turn %d record %+v", r.Operation, r.Phase, r.Turn, r.Operations)
	}
	if err := r.EndOperation(ref, sc, mod, central(t, "2026-10-02 13:00")); err == nil {
		t.Error("ended an operation that hasn't started")
	}
}

func deleteUnit(units []*Unit, id string) []*Unit {
	var out []*Unit
	for _, u := range units {
		if u.ID != id {
			out = append(out, u)
		}
	}
	return out
}

func TestUnitCardVisibility(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	r.Unit("z2").Known = true // Horde B, seen from the beach
	if _, err := BuildUnitCard(r, mod, hicks, "z2"); err == nil {
		t.Error("player opened an enemy card by its real id")
	}
	c, err := BuildUnitCard(r, mod, hicks, contactID(r, r.Unit("z2")))
	if err != nil || !c.Contact || c.Class != nil || strings.Contains(c.Unit.Name, "Horde") {
		t.Errorf("contact card leaks or fails: %v %+v", err, c)
	}
	if _, err := BuildUnitCard(r, mod, hicks, contactID(r, r.Unit("z1"))); err == nil {
		t.Error("player opened a card for an unseen enemy")
	}
	if c, err := BuildUnitCard(r, mod, ref, "z1"); err != nil || len(c.Traits) == 0 {
		t.Errorf("referee card for Horde A: %v", err)
	}
	r.Unit("m1").Operations = 4
	c, err = BuildUnitCard(r, mod, hicks, "m1")
	if err != nil || c.Level.Name != "Elite" || c.Level.Dice != 3 || c.NextLevel != nil || c.Class.Movement != 4 {
		t.Errorf("friendly card: %v %+v", err, c)
	}
}

func TestRulesHideTheBestiaryFromPlayers(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	if p := BuildRules(r, mod, hicks); len(p.Infected) != 0 || len(p.Traits) != 0 {
		t.Error("players see the infected roster")
	}
	if p := BuildRules(r, mod, ref); len(p.Infected) == 0 {
		t.Error("referee has no infected roster")
	}
}
