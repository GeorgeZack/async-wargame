package game

import (
	"testing"
	"time"

	"asyncwargame/content"
)

func central(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("America/Chicago")
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	cat, err := LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func testScenario(t *testing.T) *Scenario { return testCatalog(t).Scenarios["st-ines"] }

func testRoom(t *testing.T) (*Room, *Module, *Scenario, *Member) {
	t.Helper()
	cat := testCatalog(t)
	sc := cat.Scenarios["st-ines"]
	mod := cat.Modules[sc.Module]
	r := newRoom("ABCDEF", "Test", sc, mod, "Ref", HashToken("ref"))
	return r, mod, sc, r.Referee()
}

func startedRoom(t *testing.T, now time.Time) (*Room, *Module, *Member) {
	t.Helper()
	r, mod, _, ref := testRoom(t)
	if err := r.StartOperation(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	return r, mod, ref
}

// orderRoom is a started St. Ines room: Apone commands, Hicks reports to
// him and holds Reyes (m1), Vasquez holds Okafor (m2).
func orderRoom(t *testing.T) (*Room, *Module, *Member, *Member, *Member) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	cmd := &Member{ID: "c", Name: "Apone", Role: RoleCommander, Status: StatusApproved}
	hicks := &Member{ID: "h", Name: "Hicks", Role: RolePlayer, Status: StatusApproved, ReportsTo: "c"}
	vasq := &Member{ID: "v", Name: "Vasquez", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, cmd, hicks, vasq)
	r.Unit("m1").AssignedTo = "h"
	r.Unit("m2").AssignedTo = "v"
	return r, mod, ref, cmd, hicks
}

// publishTurn locks (if needed) and publishes the current turn.
func publishTurn(t *testing.T, r *Room, mod *Module, ref *Member, now time.Time) {
	t.Helper()
	if r.Phase == PhasePlanning {
		r.Tick(mod, r.Deadline)
	}
	if err := r.PublishTurn(ref, mod, now); err != nil {
		t.Fatal(err)
	}
}
