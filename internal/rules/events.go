package rules

import (
	"fmt"
	"math/rand/v2"
)

type EventDef struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Category  string      `json:"category"` // horde, wave, special, brood, monster, respite
	Weight    int         `json:"weight"`
	MinThreat int         `json:"minThreat,omitempty"`
	MaxThreat int         `json:"maxThreat,omitempty"`
	Requires  string      `json:"requires,omitempty"` // a unit of this class must be on the map
	Unique    string      `json:"unique,omitempty"`   // not eligible while a unit of this class is on the map
	Referee   string      `json:"referee"`            // what happens, for the referee
	Report    string      `json:"report,omitempty"`   // what players are told; empty means nothing
	Effects   []EffectDef `json:"effects"`
}

// Range is an inclusive [min, max] for a random draw.
type Range [2]int

// Draw picks a value in the range.
func (r Range) Draw(rng *rand.Rand) int {
	if r[1] <= r[0] {
		return r[0]
	}
	return r[0] + rng.IntN(r[1]-r[0]+1)
}

// EffectDef is one operation. Supported ops:
//
//	spawn   place Count new units of Class at hexes chosen by Where
//	grow    raise the strength of Count units of Classes by Percent
//	drift   move Count units of Classes up to Steps hexes Toward a target
//	reveal  make Count hidden units of Side known to players
//	threat  change the campaign threat level by Amount
//
// Count 0..0 means "all matching units" for grow and drift.
type EffectDef struct {
	Op       string   `json:"op"`
	Class    string   `json:"class,omitempty"`
	Classes  []string `json:"classes,omitempty"`
	Count    Range    `json:"count"`
	Strength Range    `json:"strength"`
	Percent  Range    `json:"percent"`
	Steps    Range    `json:"steps"`
	Scale    bool     `json:"scale,omitempty"` // multiply by the threat factor
	Known    bool     `json:"known,omitempty"` // spawned units are visible to players
	Where    Where    `json:"where"`
	Toward   string   `json:"toward,omitempty"` // "marines" or a feature kind
	Side     string   `json:"side,omitempty"`
	Within   int      `json:"within,omitempty"` // reveal: max distance from marines; 0 = anywhere
	Amount   int      `json:"amount,omitempty"`
	Turns    Range    `json:"turns"` // jam: how many turns the radios stay out
}

// Where selects candidate hexes for a spawn. Selectors narrow the whole map;
// spawns never land on water or in a hex closer than AwayFromMarines to marines.
type Where struct {
	Edge            string   `json:"edge,omitempty"` // north, south, east, west, or inland (any edge but the sea)
	Terrain         []string `json:"terrain,omitempty"`
	NearFeature     string   `json:"nearFeature,omitempty"` // within one hex of a feature of this kind
	NearClass       string   `json:"nearClass,omitempty"`   // within one hex of a unit of this class
	AwayFromMarines int      `json:"awayFromMarines,omitempty"`
}

// Validate checks an event against the module.
func (e *EventDef) Validate(m *Module) error {
	if e.Weight <= 0 {
		return fmt.Errorf("weight must be positive")
	}
	for _, c := range []string{e.Requires, e.Unique} {
		if c != "" && m.Class(c) == nil {
			return fmt.Errorf("unknown class %q", c)
		}
	}
	for i, f := range e.Effects {
		bad := func(format string, args ...any) error {
			return fmt.Errorf("effect %d (%s): %s", i, f.Op, fmt.Sprintf(format, args...))
		}
		for _, c := range append([]string{f.Class, f.Where.NearClass}, f.Classes...) {
			if c != "" && m.Class(c) == nil {
				return bad("unknown class %q", c)
			}
		}
		for _, t := range f.Where.Terrain {
			if !m.HasTerrain(t) {
				return bad("unknown terrain %q", t)
			}
		}
		switch f.Op {
		case "spawn":
			if f.Class == "" {
				return bad("class is required")
			}
			w := f.Where
			if w.Edge == "" && len(w.Terrain) == 0 && w.NearFeature == "" && w.NearClass == "" {
				return bad("where needs at least one selector")
			}
		case "grow", "drift":
			if len(f.Classes) == 0 {
				return bad("classes are required")
			}
			if f.Op == "drift" && f.Toward == "" {
				return bad("toward is required")
			}
		case "reveal":
			if m.Side(f.Side) == nil {
				return bad("unknown side %q", f.Side)
			}
		case "threat":
			if f.Amount == 0 {
				return bad("amount is required")
			}
		case "jam":
			if f.Turns[0] < 1 {
				return bad("turns must be at least 1")
			}
		default:
			return bad("unknown op")
		}
	}
	return nil
}
