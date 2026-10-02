package rules

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	squad := Party{Name: "Squad", Value: 55, Advantage: 20, Why: "cover"}
	horde := Party{Name: "Horde", Value: 40}
	cases := []struct {
		name        string
		a           Party
		da          int
		b           *Party
		db          int
		winner, deg string
	}{
		{"one succeeds", squad, 50, &horde, 70, WinnerA, DegreeClear},
		{"other succeeds", squad, 80, &horde, 30, WinnerB, DegreeClear},
		{"bigger margin wins", squad, 20, &horde, 35, WinnerA, DegreeClear}, // 55 vs 5
		{"close margins", squad, 60, &horde, 30, WinnerA, DegreeNarrow},     // 15 vs 10
		{"critical beats success", squad, 70, &horde, 3, WinnerB, DegreeDecisive},
		{"fumble loses big", squad, 40, &horde, 99, WinnerA, DegreeDecisive},
		{"both fail", squad, 90, &horde, 60, WinnerNone, ""},
		{"tie", Party{Name: "A", Value: 50}, 20, &Party{Name: "B", Value: 50}, 20, WinnerTie, ""},
		{"check succeeds", squad, 10, nil, 0, WinnerA, DegreeClear},
		{"check fails narrowly", squad, 80, nil, 0, WinnerNone, DegreeNarrow},
	}
	for _, c := range cases {
		res, err := Resolve(c.a, c.da, c.b, c.db)
		if err != nil || res.Winner != c.winner || res.Degree != c.deg {
			t.Errorf("%s: winner %s degree %s (%v)", c.name, res.Winner, res.Degree, err)
		}
	}
	if _, err := Resolve(squad, 0, nil, 0); err == nil {
		t.Error("accepted a 0 on a d100")
	}
}

func TestChanceIsClamped(t *testing.T) {
	if c := (Party{Value: 90, Advantage: 30}).Chance(); c != MaxChance {
		t.Errorf("chance %d", c)
	}
	if c := (Party{Value: 10, Advantage: -40}).Chance(); c != MinChance {
		t.Errorf("chance %d", c)
	}
}

func TestContestText(t *testing.T) {
	res, _ := Resolve(Party{Name: "Kowalski", Value: 55, Advantage: 20}, 31, &Party{Name: "the horde", Value: 40}, 62)
	got := res.Text()
	for _, want := range []string{"Kowalski 55% +20% = 75%, rolled 31 (by 44)", "the horde 40%, rolled 62 (miss)", "Kowalski wins, clear."} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing %q", got, want)
		}
	}
	// Server rolls stay in range.
	rng := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		r := Contest(rng, Party{Value: 50}, &Party{Value: 50})
		if r.A.Die < 1 || r.A.Die > 100 || r.B.Die < 1 || r.B.Die > 100 {
			t.Fatal("die out of range")
		}
	}
}
