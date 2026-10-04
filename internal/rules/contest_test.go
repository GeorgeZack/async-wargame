package rules

import (
	"math/rand/v2"
	"testing"
)

func TestResolve(t *testing.T) {
	a := Party{Name: "Okafor", Bonus: 2, Why: "trained"}
	b := Party{Name: "the horde", Bonus: 1, Why: "numbers"}
	res, err := Resolve(a, 3, &b, 8)
	if err != nil || res.A.Total != 5 || res.B.Total != 9 {
		t.Fatalf("%+v %v", res, err)
	}
	if got := res.Text(); got != "Okafor 5 (rolled 3, +2: trained); the horde 9 (rolled 8, +1: numbers)" {
		t.Errorf("text %q", got)
	}
	if got := res.Short(); got != "Okafor 5 · the horde 9" {
		t.Errorf("short %q", got)
	}
	alone, _ := Resolve(Party{Name: "Lin", Bonus: -1, Why: "dark"}, 6, nil, 0)
	if alone.A.Total != 5 || alone.B != nil || alone.Text() != "Lin 5 (rolled 6, -1: dark)" {
		t.Errorf("alone %+v %q", alone, alone.Text())
	}
	if _, err := Resolve(a, 11, nil, 0); err == nil {
		t.Error("accepted 11 on a d10")
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		r := Contest(rng, a, &b)
		if r.A.Die < 1 || r.A.Die > 10 || r.B.Die < 1 || r.B.Die > 10 {
			t.Fatal("die out of range")
		}
	}
}

func TestEqualOpponentsHaveSymmetricDiceOutcomes(t *testing.T) {
	// Exhaust the 100 possible d10 pairs rather than using a flaky random sample.
	a, b := Party{Name: "Alpha", Bonus: 2}, Party{Name: "Bravo", Bonus: 2}
	wins, losses, ties := 0, 0, 0
	for da := 1; da <= 10; da++ {
		for db := 1; db <= 10; db++ {
			result, _ := Resolve(a, da, &b, db)
			reversed, _ := Resolve(b, db, &a, da)
			margin := result.A.Total - result.B.Total
			if margin != -(reversed.A.Total - reversed.B.Total) {
				t.Fatal("side ordering changed the result")
			}
			switch {
			case margin > 0:
				wins++
			case margin < 0:
				losses++
			default:
				ties++
			}
		}
	}
	if wins != 45 || losses != 45 || ties != 10 {
		t.Fatalf("asymmetric totals: %d/%d/%d", wins, losses, ties)
	}
}
