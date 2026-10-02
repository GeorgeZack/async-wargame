package rules

import (
	"fmt"
	"math/rand/v2"
)

// Contests. Each side has a chance in percent: its skill plus any advantage
// the referee grants (or minus a handicap), kept between 5 and 95 so nothing
// is ever certain. Each side rolls d100; rolling the chance or under
// succeeds, and the margin is how far under. A roll of a tenth of the chance
// or less is a critical; 96 or more is a fumble.
//
// When both sides roll: if one succeeds, it wins; if both do, the bigger
// margin wins (a critical beats a plain success); if both fail, nobody gets
// what they wanted and the referee adds a complication. A check against no
// one is just one side's roll.
//
// The dice only say who did better and by how much. What that means in the
// story is the referee's call.

const (
	MinChance = 5
	MaxChance = 95
)

// Party is one side of a contest.
type Party struct {
	Name      string `json:"name"`            // who: "1st Rifle Squad", "the horde"
	Skill     string `json:"skill,omitempty"` // which skill, for the record
	Value     int    `json:"value"`           // skill percent
	Advantage int    `json:"advantage"`       // percent, may be negative
	Why       string `json:"why,omitempty"`   // why the advantage: "in cover, flanking"
}

// Chance is the percent this party needs to roll under.
func (p Party) Chance() int { return min(MaxChance, max(MinChance, p.Value+p.Advantage)) }

// Roll is how one party's die came up.
type Roll struct {
	Party
	Chance   int  `json:"chance"`
	Die      int  `json:"die"`
	Success  bool `json:"success"`
	Critical bool `json:"critical,omitempty"`
	Fumble   bool `json:"fumble,omitempty"`
	Margin   int  `json:"margin"` // chance minus die: positive on a success
}

func rollFor(p Party, die int) Roll {
	c := p.Chance()
	r := Roll{Party: p, Chance: c, Die: die, Success: die <= c, Margin: c - die}
	r.Critical = r.Success && die <= max(1, c/10)
	r.Fumble = !r.Success && die >= 96
	return r
}

// Winners.
const (
	WinnerA    = "a"
	WinnerB    = "b"
	WinnerNone = "none" // both failed, or a check failed
	WinnerTie  = "tie"  // both succeeded equally well
)

// Degrees, for the referee's reading.
const (
	DegreeNarrow   = "narrow"
	DegreeClear    = "clear"
	DegreeDecisive = "decisive"
)

// Result is a rolled contest or check.
type Result struct {
	A      Roll   `json:"a"`
	B      *Roll  `json:"b,omitempty"`
	Winner string `json:"winner"`
	Degree string `json:"degree,omitempty"`
}

// D100 rolls 1 to 100.
func D100(rng *rand.Rand) int { return rng.IntN(100) + 1 }

// Resolve works out a contest from dice already rolled (by the server, or
// at the table). b may be nil for a plain check.
func Resolve(a Party, dieA int, b *Party, dieB int) (Result, error) {
	if dieA < 1 || dieA > 100 || b != nil && (dieB < 1 || dieB > 100) {
		return Result{}, fmt.Errorf("%w: dice show 1 to 100", ErrInvalid)
	}
	res := Result{A: rollFor(a, dieA)}
	if b == nil {
		switch {
		case res.A.Critical || res.A.Fumble:
			res.Degree = DegreeDecisive
		case res.A.Success && res.A.Margin >= 30, !res.A.Success && -res.A.Margin >= 30:
			res.Degree = DegreeClear
		default:
			res.Degree = DegreeNarrow
		}
		res.Winner = WinnerNone
		if res.A.Success {
			res.Winner = WinnerA
		}
		return res, nil
	}
	rb := rollFor(*b, dieB)
	res.B = &rb
	ra := res.A
	switch {
	case ra.Success && !rb.Success:
		res.Winner, res.Degree = WinnerA, oneSided(ra, rb)
	case rb.Success && !ra.Success:
		res.Winner, res.Degree = WinnerB, oneSided(rb, ra)
	case !ra.Success && !rb.Success:
		res.Winner = WinnerNone
	case ra.Critical != rb.Critical:
		res.Winner, res.Degree = WinnerA, DegreeDecisive
		if rb.Critical {
			res.Winner = WinnerB
		}
	case ra.Margin == rb.Margin:
		res.Winner = WinnerTie
	default:
		res.Winner = WinnerA
		if rb.Margin > ra.Margin {
			res.Winner = WinnerB
		}
		res.Degree = DegreeNarrow
		if d := ra.Margin - rb.Margin; d >= 20 || d <= -20 {
			res.Degree = DegreeClear
		}
	}
	return res, nil
}

func oneSided(win, lose Roll) string {
	if win.Critical || lose.Fumble {
		return DegreeDecisive
	}
	return DegreeClear
}

// Contest rolls the dice and resolves.
func Contest(rng *rand.Rand, a Party, b *Party) Result {
	da, db := D100(rng), 0
	if b != nil {
		db = D100(rng)
	}
	res, _ := Resolve(a, da, b, db)
	return res
}

// Text is the result in a line: the numbers, then who came out ahead.
func (r Result) Text() string {
	s := rollText(r.A)
	if r.B != nil {
		s += " vs " + rollText(*r.B) + ". "
		switch r.Winner {
		case WinnerA:
			s += fmt.Sprintf("%s wins, %s.", r.A.Name, r.Degree)
		case WinnerB:
			s += fmt.Sprintf("%s wins, %s.", r.B.Name, r.Degree)
		case WinnerTie:
			s += "Dead even."
		default:
			s += "Both fail: a complication."
		}
		return s
	}
	switch {
	case r.A.Critical:
		return s + ": critical success."
	case r.A.Success:
		return s + fmt.Sprintf(": success, %s.", r.Degree)
	case r.A.Fumble:
		return s + ": fumble."
	}
	return s + fmt.Sprintf(": failure, %s.", r.Degree)
}

func rollText(r Roll) string {
	s := fmt.Sprintf("%s %d%%", r.Name, r.Value)
	if r.Advantage != 0 {
		s += fmt.Sprintf(" %+d%%", r.Advantage)
	}
	if r.Chance != r.Value {
		s += fmt.Sprintf(" = %d%%", r.Chance)
	}
	s += fmt.Sprintf(", rolled %d", r.Die)
	switch {
	case r.Critical:
		s += " (critical)"
	case r.Fumble:
		s += " (fumble)"
	case r.Success:
		s += fmt.Sprintf(" (by %d)", r.Margin)
	default:
		s += " (miss)"
	}
	return s
}
