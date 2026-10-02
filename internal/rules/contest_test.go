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
