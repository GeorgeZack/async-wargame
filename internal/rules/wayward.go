package rules

import (
	"fmt"
	"math/rand/v2"
)

// WaywardSkills and the derived abilities follow Core-Rules.html at
// 796f407d250e9953d811a22dba3c0182044c9e87. See docs/wayward.md for scope/credit.
type WaywardSkills struct {
	Combat   int `json:"combat"`
	Craft    int `json:"craft"`
	Lore     int `json:"lore"`
	Physical int `json:"physical"`
	Social   int `json:"social"`
	Stealth  int `json:"stealth"`
}

func (s WaywardSkills) Value(skill string) (int, bool) {
	switch skill {
	case "Combat":
		return s.Combat, true
	case "Craft":
		return s.Craft, true
	case "Lore":
		return s.Lore, true
	case "Physical":
		return s.Physical, true
	case "Social":
		return s.Social, true
	case "Stealth":
		return s.Stealth, true
	}
	return 0, false
}

// This release supports human, narrative-archetype pregens without spells or
// Combat Moves. These are restrictions of the playtest, not of Wayward itself.
type WaywardCharacter struct {
	Archetype string        `json:"archetype"`
	Armor     int           `json:"armor"`
	Skills    WaywardSkills `json:"skills"`
}

func (c WaywardCharacter) Stamina() int     { return 14 + 3*c.Skills.Physical }
func (c WaywardCharacter) Defense() int     { return 8 + (c.Skills.Combat+1)/2 }
func (c WaywardCharacter) Connections() int { return 3 + c.Skills.Social }
func (c WaywardCharacter) Validate() error {
	if c.Armor != 0 && c.Armor != 2 {
		return fmt.Errorf("%w: exploration pregens support no armor or light armor", ErrInvalid)
	}
	sum := 0
	for _, s := range []string{"Combat", "Craft", "Lore", "Physical", "Social", "Stealth"} {
		v, _ := c.Skills.Value(s)
		if v < 0 || v > 3 {
			return fmt.Errorf("%w: starting Skills must be 0–3", ErrInvalid)
		}
		sum += v
	}
	if sum != 5 || c.Skills.Combat > 1 || c.Skills.Lore != 0 {
		return fmt.Errorf("%w: exploration pregens need five Skill points, Combat at most 1 and Lore 0; Combat Moves and spells are not implemented", ErrInvalid)
	}
	switch c.Archetype {
	case "Burglar", "Explorer", "Merchant", "Bard", "Scoundrel", "Tomb Robber":
	default:
		return fmt.Errorf("%w: unsupported exploration Archetype", ErrInvalid)
	}
	return nil
}

type WaywardThrow struct {
	Dice     [2]int `json:"dice"`
	Skill    int    `json:"skill"`
	Target   int    `json:"target"`
	Total    int    `json:"total"`
	Success  bool   `json:"success"`
	Critical bool   `json:"critical"`
}

func ResolveWayward(skill, target, a, b int) (WaywardThrow, error) {
	if a < 1 || a > 6 || b < 1 || b > 6 || skill < 0 || skill > 5 || target < 6 || target > 14 || target%2 != 0 {
		return WaywardThrow{}, fmt.Errorf("%w: invalid Wayward task throw", ErrInvalid)
	}
	n := a + b
	return WaywardThrow{Dice: [2]int{a, b}, Skill: skill, Target: target, Total: n + skill, Success: n == 12 || n != 2 && n+skill >= target, Critical: n == 12}, nil
}

func RollWayward(skill, target int) (WaywardThrow, error) {
	return ResolveWayward(skill, target, rand.IntN(6)+1, rand.IntN(6)+1)
}

func (r WaywardThrow) Text() string {
	result := "failure"
	if r.Success {
		result = "success"
	}
	if r.Critical {
		result = "critical success"
	}
	if r.Dice == [2]int{1, 1} {
		result = "natural 2: failure"
	}
	return fmt.Sprintf("2d6 (%d + %d) %+d = %d vs %d+: %s", r.Dice[0], r.Dice[1], r.Skill, r.Total, r.Target, result)
}
