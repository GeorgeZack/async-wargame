package rules

import (
	"math/rand/v2"
	"testing"
)

func testCombat() *Combat {
	return &Combat{Die: 6, Success: 4, UnitDice: "attack",
		Groups: &PerStrength{Per: 20, Dice: 1, Max: 6, Label: "Numbers"},
		Modifiers: []Modifier{
			{When: "holds_cover", Label: "Cover", PerCover: 1},
			{When: "across_barrier", Label: "Across", Dice: -2},
			{When: "unseen", Label: "Ambush", Dice: 2},
			{When: "missed_orders", Label: "No orders", Stat: "armor"},
			{When: "out_of_ammo", Label: "No ammo", Halve: true},
		},
		Outcomes: []Outcome{{Min: 0, Max: 0, Result: "even"}, {Min: 1, Max: 2, Result: "small"}, {Min: 3, Max: 0, Result: "big"}},
	}
}

func TestPoolFactors(t *testing.T) {
	c := testCombat()
	squad := &Class{Name: "Squad", Attack: 3, Armor: 2}
	horde := &Class{Name: "Horde", Attack: 4, Kind: "group"}
	cases := []struct {
		name string
		side SideState
		want int
	}{
		{"squad in cover with experience", SideState{Fighters: []Fighter{{Name: "1st", Class: squad, XPDice: 2}}, Holding: true, Cover: 2}, 3 + 2 + 2},
		{"squad without orders defends with armor", SideState{Fighters: []Fighter{{Name: "1st", Class: squad, Missed: true}}}, 2},
		{"squad out of ammo", SideState{Fighters: []Fighter{{Name: "1st", Class: squad, OutOfAmmo: true}}}, 1},
		{"horde of 90 from hiding", SideState{Fighters: []Fighter{{Name: "A", Class: horde, Strength: 90}}, Unseen: true}, 4 + 4 + 2},
		{"crossing a river", SideState{Fighters: []Fighter{{Name: "1st", Class: squad}}, Across: true}, 1},
		{"never below zero", SideState{Fighters: []Fighter{{Name: "C", Class: &Class{}}}, Across: true}, 0},
		{"a crowd adds no numbers", SideState{Fighters: []Fighter{{Name: "Crowd", Class: &Class{Kind: "group"}, Strength: 120}}}, 0},
	}
	for _, tc := range cases {
		if got := c.Pool(tc.side); got.Dice != tc.want {
			t.Errorf("%s: %d dice (%+v), want %d", tc.name, got.Dice, got.Factors, tc.want)
		}
	}
}

func TestOutcomeTableAndRoll(t *testing.T) {
	c := testCombat()
	for margin, want := range map[int]string{0: "even", 1: "small", 2: "small", 3: "big", 12: "big"} {
		if got := c.OutcomeFor(margin).Result; got != want {
			t.Errorf("margin %d: %q, want %q", margin, got, want)
		}
	}
	faces, hits := c.Roll(rand.New(rand.NewPCG(1, 1)), 30)
	n := 0
	for _, f := range faces {
		if f < 1 || f > 6 {
			t.Fatalf("face %d", f)
		}
		if f >= 4 {
			n++
		}
	}
	if len(faces) != 30 || n != hits {
		t.Errorf("%d faces, %d hits, counted %d", len(faces), hits, n)
	}
	if got := (Outcome{Min: 7}).MarginText(); got != "7+" {
		t.Errorf("margin text %q", got)
	}
}

func TestCombatValidation(t *testing.T) {
	c := testCombat()
	c.Modifiers = append(c.Modifiers, Modifier{When: "full_moon"})
	if err := c.Validate(); err == nil {
		t.Error("accepted an unknown condition")
	}
}
