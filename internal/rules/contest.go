package rules

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Rolls. When a situation is in doubt, each side rolls a d10 and adds one
// bonus: everything that matters summed into a single number (how good the
// unit is, how experienced, the situation, an ambush, the weather). The
// result is just the two totals: "Okafor 5, the horde 9". There's no winner
// computed and no table to read: the referee (or the AI, for the referee)
// looks at the totals, at what each side was trying to do, and decides what
// happens. A side can also roll alone, against the situation.

// Party is one side of a roll.
type Party struct {
	Name  string `json:"name"`          // who: "Okafor", "the horde"
	Bonus int    `json:"bonus"`         // everything that matters, summed
	Why   string `json:"why,omitempty"` // what went into it: "trained +2, ambush +3"
}

// Roll is how one side's die came up.
type Roll struct {
	Party
	Die   int `json:"die"`
	Total int `json:"total"`
}

// Result is a rolled situation: one side, or two.
type Result struct {
	A Roll  `json:"a"`
	B *Roll `json:"b,omitempty"`
}

// D10 rolls 1 to 10.
func D10(rng *rand.Rand) int { return rng.IntN(10) + 1 }

// Resolve totals dice already rolled (by the server, or at the table). b
// may be nil when a side rolls alone.
func Resolve(a Party, dieA int, b *Party, dieB int) (Result, error) {
	if dieA < 1 || dieA > 10 || b != nil && (dieB < 1 || dieB > 10) {
		return Result{}, fmt.Errorf("%w: a d10 shows 1 to 10", ErrInvalid)
	}
	res := Result{A: Roll{Party: a, Die: dieA, Total: dieA + a.Bonus}}
	if b != nil {
		res.B = &Roll{Party: *b, Die: dieB, Total: dieB + b.Bonus}
	}
	return res, nil
}

// Contest rolls the dice and totals them.
func Contest(rng *rand.Rand, a Party, b *Party) Result {
	da, db := D10(rng), 0
	if b != nil {
		db = D10(rng)
	}
	res, _ := Resolve(a, da, b, db)
	return res
}

// Text is the result in a line: "Okafor 5 (rolled 3, +2: trained), the
// horde 9 (rolled 8, +1: numbers)".
func (r Result) Text() string {
	parts := []string{rollText(r.A)}
	if r.B != nil {
		parts = append(parts, rollText(*r.B))
	}
	return strings.Join(parts, "; ")
}

// Short is just the totals: "Okafor 5 · the horde 9".
func (r Result) Short() string {
	s := fmt.Sprintf("%s %d", r.A.Name, r.A.Total)
	if r.B != nil {
		s += fmt.Sprintf(" · %s %d", r.B.Name, r.B.Total)
	}
	return s
}

func rollText(r Roll) string {
	s := fmt.Sprintf("%s %d (rolled %d", r.Name, r.Total, r.Die)
	if r.Bonus != 0 {
		s += fmt.Sprintf(", %+d", r.Bonus)
		if r.Why != "" {
			s += ": " + r.Why
		}
	}
	return s + ")"
}
