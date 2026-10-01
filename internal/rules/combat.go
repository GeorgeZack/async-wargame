package rules

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// Combat is a module's dice-pool procedure: each side's pool is built from
// its units and the situation, both pools are rolled, successes cancel, and
// the margin is read on an outcome table. The engine knows a fixed set of
// situation conditions (see Modifier.When); the module decides what each is
// worth and what every outcome does.
type Combat struct {
	Die       int          `json:"die"`
	Success   int          `json:"success"`  // a die showing this or more is a success
	UnitDice  string       `json:"unitDice"` // the class stat each unit adds: "attack", "armor", ...
	Procedure []string     `json:"procedure"`
	Groups    *PerStrength `json:"groups,omitempty"`
	Modifiers []Modifier   `json:"modifiers"`
	Outcomes  []Outcome    `json:"outcomes"`
	Infection *Infection   `json:"infection,omitempty"`
	Ammo      int          `json:"ammo"` // ammunition each fighting unit spends per encounter, of every type it carries
}

// PerStrength adds dice for numbers in "group" units (hordes, crowds).
type PerStrength struct {
	Per   int    `json:"per"`
	Dice  int    `json:"dice"`
	Max   int    `json:"max"`
	Label string `json:"label"`
}

// Modifier.When is one of:
//
//	holds_cover     the side held its ground: +PerCover dice per point of cover in its hex
//	across_barrier  the sides are separated by a river, ridge, or wall side without a crossing
//	unseen          the side was hidden when the fight began (an ambush)
//	missed_orders   a unit without orders this turn: uses Stat instead of the usual one
//	out_of_ammo     a unit with no ammunition left: Halve its dice
//	trait:<id>      a unit with that trait
type Modifier struct {
	When     string `json:"when"`
	Label    string `json:"label"`
	Dice     int    `json:"dice,omitempty"`
	PerCover int    `json:"perCover,omitempty"`
	Stat     string `json:"stat,omitempty"`
	Halve    bool   `json:"halve,omitempty"`
}

type Outcome struct {
	Min    int     `json:"min"`
	Max    int     `json:"max"` // 0 with Min > 0 means "or more"
	Result string  `json:"result"`
	Loser  Effects `json:"loser"`
	Winner Effects `json:"winner"`
}

// Effects of an outcome on one side's units.
type Effects struct {
	Casualties int  `json:"casualties,omitempty"` // percent of strength lost
	Retreat    int  `json:"retreat,omitempty"`    // hexes to fall back
	Suppressed bool `json:"suppressed,omitempty"`
}

// Infection: when Side wins against Victims, a share of the victims lost
// join Side's largest unit.
type Infection struct {
	Side    string  `json:"side"`
	Victims string  `json:"victims"`
	Rate    float64 `json:"rate"`
	Label   string  `json:"label"`
}

// MarginText renders an outcome's margin range: "0", "1–2", "7+".
func (o Outcome) MarginText() string {
	switch {
	case o.Max == 0 && o.Min > 0:
		return fmt.Sprintf("%d+", o.Min)
	case o.Max == o.Min:
		return fmt.Sprint(o.Min)
	}
	return fmt.Sprintf("%d–%d", o.Min, o.Max)
}

// Fighter is one unit in an encounter, as the engine sees it.
type Fighter struct {
	Name      string
	Class     *Class
	Strength  int
	XPDice    int
	Missed    bool // had no orders this turn
	OutOfAmmo bool
}

// Side is one side of an encounter.
type SideState struct {
	Fighters []Fighter
	Holding  bool // didn't move into the fight
	Cover    int  // cover of the hex it holds
	Across   bool // separated from the enemy by a barrier
	Unseen   bool // hidden when the fight began
}

type Factor struct {
	Label string `json:"label"`
	Dice  int    `json:"dice"`
}

type Pool struct {
	Dice    int      `json:"dice"`
	Factors []Factor `json:"factors"`
}

func (p *Pool) add(label string, dice int) {
	if dice == 0 {
		return
	}
	for i := range p.Factors {
		if p.Factors[i].Label == label {
			p.Factors[i].Dice += dice
			p.Dice += dice
			return
		}
	}
	p.Factors = append(p.Factors, Factor{label, dice})
	p.Dice += dice
}

func stat(cl *Class, name string) int {
	switch name {
	case "attack":
		return cl.Attack
	case "armor":
		return cl.Armor
	case "movement":
		return cl.Movement
	case "range":
		return cl.Range
	}
	return 0
}

func (c *Combat) modifier(when string) *Modifier {
	for i := range c.Modifiers {
		if c.Modifiers[i].When == when {
			return &c.Modifiers[i]
		}
	}
	return nil
}

// Pool builds one side's suggested dice pool, listing every factor.
func (c *Combat) Pool(s SideState) Pool {
	var p Pool
	for _, f := range s.Fighters {
		statName := c.UnitDice
		if m := c.modifier("missed_orders"); m != nil && f.Missed && m.Stat != "" {
			statName = m.Stat
		}
		dice := stat(f.Class, statName)
		label := fmt.Sprintf("%s (%s %d)", f.Name, statName, dice)
		if m := c.modifier("out_of_ammo"); m != nil && f.OutOfAmmo && m.Halve {
			dice /= 2
			label += ", out of ammunition"
		}
		p.add(label, dice)
		// Numbers count only for units that fight: a crowd of civilians isn't a horde.
		if g := c.Groups; g != nil && g.Per > 0 && stat(f.Class, c.UnitDice) > 0 && (f.Class.Kind == "" || f.Class.Kind == "group") {
			p.add(g.Label, min(g.Max, f.Strength/g.Per*g.Dice))
		}
		if f.XPDice > 0 {
			p.add("Experience", f.XPDice)
		}
		for _, m := range c.Modifiers {
			if t, ok := strings.CutPrefix(m.When, "trait:"); ok && slices.Contains(f.Class.Traits, t) {
				p.add(m.Label, m.Dice)
			}
		}
	}
	if m := c.modifier("holds_cover"); m != nil && s.Holding && s.Cover > 0 {
		p.add(m.Label, m.PerCover*s.Cover)
	}
	if m := c.modifier("across_barrier"); m != nil && s.Across {
		p.add(m.Label, m.Dice)
	}
	if m := c.modifier("unseen"); m != nil && s.Unseen {
		p.add(m.Label, m.Dice)
	}
	p.Dice = max(0, p.Dice)
	return p
}

// Roll throws dice and counts successes.
func (c *Combat) Roll(rng *rand.Rand, dice int) (faces []int, hits int) {
	for range dice {
		f := rng.IntN(c.Die) + 1
		faces = append(faces, f)
		if f >= c.Success {
			hits++
		}
	}
	return faces, hits
}

// OutcomeFor reads a margin (always zero or more) on the outcome table.
func (c *Combat) OutcomeFor(margin int) Outcome {
	for _, o := range c.Outcomes {
		if margin >= o.Min && (margin <= o.Max || o.Max == 0 && o.Min > 0) {
			return o
		}
	}
	return Outcome{Result: "No result on the table."}
}

// Validate checks the combat section of a module.
func (c *Combat) Validate() error {
	if c.Die < 2 || c.Success < 1 || c.Success > c.Die {
		return bad("combat: die and success must make sense")
	}
	if c.UnitDice == "" {
		c.UnitDice = "attack"
	}
	known := []string{"holds_cover", "across_barrier", "unseen", "missed_orders", "out_of_ammo"}
	for _, m := range c.Modifiers {
		if !slices.Contains(known, m.When) && !strings.HasPrefix(m.When, "trait:") {
			return bad("combat: unknown modifier condition %q", m.When)
		}
	}
	if len(c.Outcomes) == 0 {
		return bad("combat: needs an outcome table")
	}
	return nil
}
