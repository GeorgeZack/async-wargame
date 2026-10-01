package game

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// quietRoom is a started room with only the given units on the map.
func quietRoom(t *testing.T, units ...*Unit) (*Room, *Module, *Member, *Member) {
	r, mod, ref, _, hicks := orderRoom(t)
	r.Units = units
	return r, mod, ref, hicks
}

func TestEscortAndEvacuation(t *testing.T) {
	// Survivors one hex inland from Beachhead Alpha, a squad with them, and a dropship offshore.
	r, mod, ref, hicks := quietRoom(t,
		&Unit{ID: "m1", Name: "1st Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 6, Row: 7, Strength: 10, Known: true, AssignedTo: "h"},
		&Unit{ID: "c1", Name: "Harbor staff", Class: "civilian_group", Side: "civilians", Col: 6, Row: 7, Strength: 15, Known: true},
		&Unit{ID: "d1", Name: "UNS Halyard", Class: "dropship", Side: "marines", Col: 3, Row: 7, Strength: 12, Known: true},
	)
	r.Stats.CiviliansStart = 15
	now := central(t, "2026-09-30 13:00")
	beach := HexID(4, 7)
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "evacuate", Dest: beach, Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	var civMoved bool
	for _, m := range r.Resolution.Moves {
		civMoved = civMoved || m.Unit == "c1" && m.To() == [2]int{4, 7}
	}
	if !civMoved {
		t.Fatalf("the civilians didn't go with their escort: %+v", r.Resolution.Moves)
	}
	publishTurn(t, r, mod, ref, now)
	if r.Stats.Evacuated != 15 || r.Unit("c1") != nil {
		t.Errorf("evacuated %d; civilians still on the map: %v", r.Stats.Evacuated, r.Unit("c1") != nil)
	}
	if !strings.Contains(strings.Join(r.Reports[0].Players, " "), "boarded UNS Halyard") {
		t.Errorf("report: %v", r.Reports[0].Players)
	}
}

func TestResupplyAtTheBeachhead(t *testing.T) {
	r, mod, ref, hicks := quietRoom(t,
		&Unit{ID: "m1", Name: "1st Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 5, Row: 7, Strength: 10, Known: true, AssignedTo: "h",
			Supply: map[string]int{"rifle": 0, "grenade": 0}},
	)
	if !outOfAmmo(mod.Class("rifle_squad"), r.Unit("m1")) {
		t.Fatal("setup: squad should be dry")
	}
	now := central(t, "2026-09-30 13:00")
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "resupply", Dest: HexID(4, 7), Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	if u := r.Unit("m1"); u.Supply != nil || outOfAmmo(mod.Class("rifle_squad"), u) {
		t.Errorf("squad didn't resupply: %v", u.Supply)
	}
}

func TestTheDeadRise(t *testing.T) {
	r, mod, ref, _ := quietRoom(t,
		&Unit{ID: "c1", Name: "Holdouts", Class: "civilian_group", Side: "civilians", Col: 12, Row: 3, Strength: 40, Known: true},
		&Unit{ID: "z1", Name: "Horde", Class: "horde", Side: "infected", Col: 12, Row: 4, Strength: 60},
	)
	r.Tick(mod, r.Deadline)
	if len(r.Resolution.Encounters) != 1 {
		t.Fatalf("encounters: %+v", r.Resolution.Encounters)
	}
	e := r.Resolution.Encounters[0]
	r.settle(mod, e, rand.New(rand.NewPCG(1, 1)), nil, &[2]int{0, 7}, "civilians overrun") // the dead win by 7
	if err := r.NextTurn(ref, mod, central(t, "2026-09-30 19:00")); err != nil {
		t.Fatal(err)
	}
	if r.Unit("c1") != nil || r.Stats.CiviliansLost != 40 {
		t.Errorf("civilians survived a decisive loss: lost %d", r.Stats.CiviliansLost)
	}
	if z := r.Unit("z1"); z.Strength != 60+20 {
		t.Errorf("horde strength %d, want 80 (half the dead rise)", z.Strength)
	}
}

func TestMonstersSurviveSmallLossesAndUnknownFightsStaySecret(t *testing.T) {
	r, mod, ref, _ := quietRoom(t,
		&Unit{ID: "m4", Name: "Corpsman Team", Class: "medic_team", Side: "marines", Col: 5, Row: 11, Strength: 4, Known: true},
		&Unit{ID: "z8", Name: "Brute", Class: "brute", Side: "infected", Col: 7, Row: 11, Strength: 1},
		&Unit{ID: "c9", Name: "Secret holdouts", Class: "civilian_group", Side: "civilians", Col: 20, Row: 1, Strength: 10},
		&Unit{ID: "z9", Name: "Far horde", Class: "horde", Side: "infected", Col: 20, Row: 2, Strength: 40},
	)
	r.Tick(mod, r.Deadline)
	if len(r.Resolution.Encounters) != 2 {
		t.Fatalf("want the brute fight and the hidden fight, got %d", len(r.Resolution.Encounters))
	}
	for _, e := range r.Resolution.Encounters {
		if slices.Contains(e.Enemy, "z8") {
			r.settle(mod, e, rand.New(rand.NewPCG(1, 1)), nil, &[2]int{0, 3}, "brute wins by 3") // winner takes 5%
		} else {
			r.settle(mod, e, rand.New(rand.NewPCG(1, 1)), nil, &[2]int{0, 4}, "")
		}
	}
	if err := r.NextTurn(ref, mod, central(t, "2026-09-30 19:00")); err != nil {
		t.Fatal(err)
	}
	if r.Unit("z8") == nil {
		t.Error("a winning brute died of a 5% loss")
	}
	for _, l := range r.Reports[0].Players {
		if strings.Contains(l, "Secret holdouts") || strings.Contains(l, "2002") {
			t.Errorf("players heard about a fight they couldn't know of: %q", l)
		}
	}
}
