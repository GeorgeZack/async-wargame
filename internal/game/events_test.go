package game

import (
	"reflect"
	"strings"
	"testing"

	"asyncwargame/content"
)

func testRoom(t *testing.T) (*Room, *Module, *Scenario, *Member) {
	t.Helper()
	cat, err := LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	sc := cat.Scenarios["dark-coast-rescue"]
	mod := cat.Modules[sc.Module]
	r := newRoom("ABCDEF", "Test", sc, mod, "Ref", HashToken("ref"))
	return r, mod, sc, r.Referee()
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
				switch e.Op {
				case "spawn":
					if mod.TerrainAt(r.Map.TerrainCode(e.Col, e.Row)).Water() {
						t.Errorf("%s: spawn in water at %s", def.ID, HexID(e.Col, e.Row))
					}
					if _, d := nearest(e.Col, e.Row, marines); d < 1 {
						t.Errorf("%s: spawn on top of marines at %s", def.ID, HexID(e.Col, e.Row))
					}
				case "drift":
					if mod.TerrainAt(r.Map.TerrainCode(e.ToCol, e.ToRow)).Water() {
						t.Errorf("%s: drift into water", def.ID)
					}
					if _, d := nearest(e.ToCol, e.ToRow, marines); d < 1 {
						t.Errorf("%s: drift into a marine hex", def.ID)
					}
				}
			}
			if strings.Contains(ev.Report, "{") {
				t.Errorf("%s: unfilled report placeholder: %q", def.ID, ev.Report)
			}
			r.Events = nil // stay under the open-proposal limit
		}
	}
}

func TestProposalsAreReproducibleFromSeed(t *testing.T) {
	r, mod, _, _ := testRoom(t)
	r.Threat = 4
	def := mod.Event("great_wave")
	a := r.propose(def, rngFor(42), mod)
	b := r.propose(def, rngFor(42), mod)
	if !reflect.DeepEqual(a.Effects, b.Effects) {
		t.Error("same seed gave different effects")
	}
}

func TestRandomRollRespectsEligibility(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	r.Threat = 1
	for range 200 {
		ev, err := r.RollEvent(ref, mod, "")
		if err != nil {
			t.Fatal(err)
		}
		if d := mod.Event(ev.Def); !r.Eligible(d) {
			t.Fatalf("rolled ineligible event %s at threat 1", ev.Def)
		}
		r.Events = nil
	}
}

func TestApplyHiddenSpawnTellsPlayersOnlyTheReport(t *testing.T) {
	r, mod, sc, ref := testRoom(t)
	r.Threat = 3
	player := &Member{ID: "p1", Name: "Hicks", Role: RolePlayer, Status: StatusApproved, Briefed: true}
	r.Members = append(r.Members, player)

	if _, err := r.RollEvent(player, mod, "brute"); err != ErrForbidden {
		t.Fatalf("player rolled an event: %v", err)
	}
	ev, err := r.RollEvent(ref, mod, "brute")
	if err != nil || len(ev.Effects) != 1 {
		t.Fatalf("roll: %v, %d effects", err, len(ev.Effects))
	}
	before := len(r.Units)
	if err := r.ApplyEvent(ref, mod, ev.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.Units) != before+1 || r.Units[before].Known {
		t.Fatal("brute was not spawned hidden")
	}
	if err := r.ApplyEvent(ref, mod, ev.ID); err == nil {
		t.Error("applied the same event twice")
	}

	v := BuildView(r, sc, mod, player)
	for _, u := range v.Data.Units {
		for _, uv := range u {
			if uv.ID == r.Units[before].ID {
				t.Error("player can see the hidden brute")
			}
		}
	}
	var sitrep bool
	for _, e := range v.Log {
		if e.Kind == "sitrep" && strings.Contains(e.Text, "smashes through a wall") {
			sitrep = true
		}
		if strings.Contains(e.Text, "Brute") {
			t.Errorf("player log names the brute: %q", e.Text)
		}
	}
	if !sitrep {
		t.Error("player did not get the sitrep")
	}
	if len(v.Proposals) != 0 || len(v.EventOptions) != 0 {
		t.Error("player view includes referee event controls")
	}
}

func TestKnownMonsterAppearsAsContactSignature(t *testing.T) {
	r, mod, sc, _ := testRoom(t)
	player := &Member{ID: "p1", Name: "Hicks", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, player)
	r.Unit("z8").Known = true // the brute
	v := BuildView(r, sc, mod, player)
	var found bool
	for _, us := range v.Data.Units {
		for _, u := range us {
			if u.ID == contactID(r, r.Unit("z8")) {
				found = true
				if u.ClassName != "Large contact" || u.Abbr != "!" || u.Traits != "" || strings.Contains(u.Name, "Brute") {
					t.Errorf("brute leaks identity to players: %+v", u)
				}
			}
		}
	}
	if !found {
		t.Error("known brute not shown as a contact")
	}
}

func TestDriftSkipsUnitsThatMovedSinceRoll(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	ev, err := r.RollEvent(ref, mod, "follow_noise")
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Effects) == 0 {
		t.Skip("no drift this roll")
	}
	u := r.Unit(ev.Effects[0].UnitID)
	u.Col, u.Row = 20, 3 // moved by something else
	if err := r.ApplyEvent(ref, mod, ev.ID); err != nil {
		t.Fatal(err)
	}
	if u.Col != 20 || u.Row != 3 {
		t.Error("stale drift was applied")
	}
	if last := r.Log[len(r.Log)-1]; !strings.Contains(last.Text, "skipped") && !strings.Contains(r.Log[len(r.Log)-2].Text, "skipped") {
		t.Error("skipped effect not recorded")
	}
}

func TestThreatClamps(t *testing.T) {
	r, _, _, ref := testRoom(t)
	for range 10 {
		r.SetThreat(ref, 1)
	}
	if r.Threat != MaxThreat {
		t.Errorf("threat = %d", r.Threat)
	}
	for range 10 {
		r.SetThreat(ref, -1)
	}
	if r.Threat != MinThreat {
		t.Errorf("threat = %d", r.Threat)
	}
}

func TestReportCountsArePluralized(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	r.Unit("z3").Col, r.Unit("z3").Row = 7, 5 // Stragglers near the beach
	for range 20 {
		ev, err := r.RollEvent(ref, mod, "drone_feed")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(ev.Report, "1 contacts") || strings.Contains(ev.Report, "{") {
			t.Fatalf("bad report: %q", ev.Report)
		}
		r.Events = nil
	}
}
