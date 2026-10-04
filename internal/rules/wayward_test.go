package rules

import "testing"

func TestWaywardDistribution(t *testing.T) {
	for _, target := range []int{6, 8, 10, 12, 14} {
		wins, crits := 0, 0
		for a := 1; a <= 6; a++ {
			for b := 1; b <= 6; b++ {
				r, err := ResolveWayward(0, target, a, b)
				if err != nil {
					t.Fatal(err)
				}
				if r.Success {
					wins++
				}
				if r.Critical {
					crits++
				}
				if r.Total != a+b {
					t.Fatal("incorrect sum")
				}
			}
		}
		want := map[int]int{6: 26, 8: 15, 10: 6, 12: 1, 14: 1}[target]
		if wins != want || crits != 1 {
			t.Fatalf("target %d: wins=%d critical=%d", target, wins, crits)
		}
	}
	r, _ := ResolveWayward(5, 6, 1, 1)
	if r.Success {
		t.Fatal("natural 2 must fail even when total beats target")
	}
	for _, x := range [][4]int{{0, 8, 0, 1}, {0, 8, 7, 1}, {0, 7, 1, 1}, {-1, 8, 1, 1}, {6, 8, 1, 1}} {
		if _, err := ResolveWayward(x[0], x[1], x[2], x[3]); err == nil {
			t.Fatal("invalid throw accepted", x)
		}
	}
}

func TestWaywardPregen(t *testing.T) {
	c := WaywardCharacter{Archetype: "Burglar", Skills: WaywardSkills{Stealth: 3, Physical: 1, Craft: 1}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Stamina() != 17 || c.Defense() != 8 || c.Connections() != 3 {
		t.Fatal("incorrect derived abilities")
	}
	c.Skills.Combat = 1
	if c.Validate() == nil {
		t.Fatal("six starting Skill points accepted")
	}
}
