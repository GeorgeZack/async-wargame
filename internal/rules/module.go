// Package rules holds a game module's schema and the dice. A module is data:
// sides, skills, unit templates, map art, random events, and the rules text
// players read. The referee interprets everything else: the dice only say
// how well each side did, and the referee (or the AI assistant, with the
// referee's approval) says what that means in the story.
//
// The campaign server (package game) depends on this package, never the
// other way round, so a new setting can be written without touching it.
package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalid marks a request or module that breaks the rules.
var ErrInvalid = errors.New("bad request")

// Module is a versioned setting.
type Module struct {
	ID         string      `json:"id"`
	Version    string      `json:"version"`
	Name       string      `json:"name"`
	Intro      string      `json:"intro,omitempty"`
	Scale      Scale       `json:"scale"`
	Rules      []Section   `json:"rules"`      // the rules page, in the setting's words
	Skills     []Skill     `json:"skills"`     // what units can be good at, in percent
	Advantages []Advantage `json:"advantages"` // suggested modifiers for contests
	Sides      []Side      `json:"sides"`
	Classes    []Class     `json:"classes"` // unit templates
	Terrain    []Terrain   `json:"terrain"` // map art
	EdgeTypes  []EdgeType  `json:"edges"`
	Crossings  []Crossing  `json:"crossings"`
	Events     []EventDef  `json:"events"`

	terrainByCode map[byte]*Terrain
	classByID     map[string]*Class
	sideByID      map[string]*Side
}

// Scale is what a hex and a turn mean in the fiction.
type Scale struct {
	Hex  string `json:"hex"`
	Turn string `json:"turn"`
}

// Section is part of the rules page.
type Section struct {
	Title string   `json:"title"`
	Text  []string `json:"text"` // paragraphs
}

// Skill is something a unit can be good at, rated in percent.
type Skill struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

// Advantage is a modifier the referee might apply to one side of a contest.
type Advantage struct {
	Name  string `json:"name"`
	Value int    `json:"value"` // percent; negative for a handicap
	Note  string `json:"note,omitempty"`
}

type Side struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Playable bool   `json:"playable"`
	Enemy    bool   `json:"enemy"`
}

// Class is a unit template: what a unit of this kind starts with.
type Class struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Abbr   string `json:"abbr"`
	Side   string `json:"side"`
	Kind   string `json:"kind,omitempty"`   // "group" (strength is a headcount) or a single "special", "monster", "nest", "vehicle"
	Symbol string `json:"symbol,omitempty"` // counter art
	// Strength is the usual headcount for a new unit of this kind.
	Strength    int            `json:"strength,omitempty"`
	Skills      map[string]int `json:"skills"`
	Description string         `json:"description,omitempty"`
	Kit         string         `json:"kit,omitempty"`     // what it usually carries, in a sentence
	Contact     *Contact       `json:"contact,omitempty"` // how players see it before it's identified
}

type Contact struct {
	Abbr  string `json:"abbr"`
	Label string `json:"label"`
}

func (c *Class) IsGroup() bool { return c.Kind == "" || c.Kind == "group" }

// Terrain is how a hex is drawn and what it's called.
type Terrain struct {
	Code    string `json:"code"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsWater bool   `json:"water,omitempty"`
	Note    string `json:"note,omitempty"`
}

func (t *Terrain) Water() bool { return t.IsWater }

// EdgeType is a feature drawn along hex sides: a river, a ridge, a wall.
type EdgeType struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Blocks    bool     `json:"blocks,omitempty"` // a real obstacle; random events route around it
	Crossings []string `json:"crossings"`        // crossings allowed on this edge
	Note      string   `json:"note,omitempty"`
}

type Crossing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

func (m *Module) TerrainAt(code byte) *Terrain { return m.terrainByCode[code] }
func (m *Module) Class(id string) *Class       { return m.classByID[id] }
func (m *Module) Side(id string) *Side         { return m.sideByID[id] }

func (m *Module) Skill(id string) *Skill {
	for i := range m.Skills {
		if m.Skills[i].ID == id {
			return &m.Skills[i]
		}
	}
	return nil
}

func (m *Module) EdgeType(id string) *EdgeType {
	for i := range m.EdgeTypes {
		if m.EdgeTypes[i].ID == id {
			return &m.EdgeTypes[i]
		}
	}
	return nil
}

func (m *Module) Crossing(id string) *Crossing {
	for i := range m.Crossings {
		if m.Crossings[i].ID == id {
			return &m.Crossings[i]
		}
	}
	return nil
}

func (m *Module) CheckEdge(typ, crossing string) error {
	et := m.EdgeType(typ)
	if et == nil {
		return fmt.Errorf("%w: unknown edge type %q", ErrInvalid, typ)
	}
	if crossing != "" && !slices.Contains(et.Crossings, crossing) {
		return fmt.Errorf("%w: a %s can't have a %s", ErrInvalid, strings.ToLower(et.Name), crossing)
	}
	return nil
}

func (m *Module) Event(id string) *EventDef {
	for i := range m.Events {
		if m.Events[i].ID == id {
			return &m.Events[i]
		}
	}
	return nil
}

func (m *Module) HasTerrain(id string) bool {
	return slices.ContainsFunc(m.Terrain, func(t Terrain) bool { return t.ID == id })
}

// Index builds lookups and validates the module.
func (m *Module) Index() error {
	m.terrainByCode = map[byte]*Terrain{}
	for i := range m.Terrain {
		t := &m.Terrain[i]
		if len(t.Code) != 1 {
			return fmt.Errorf("terrain %q: code must be one character", t.ID)
		}
		m.terrainByCode[t.Code[0]] = t
	}
	m.sideByID = map[string]*Side{}
	for i := range m.Sides {
		m.sideByID[m.Sides[i].ID] = &m.Sides[i]
	}
	if len(m.Skills) == 0 {
		return fmt.Errorf("a module needs at least one skill")
	}
	seenSkill := map[string]bool{}
	for _, s := range m.Skills {
		if s.ID == "" || seenSkill[s.ID] {
			return fmt.Errorf("skill %q: missing or duplicate id", s.ID)
		}
		seenSkill[s.ID] = true
	}
	m.classByID = map[string]*Class{}
	for i := range m.Classes {
		cl := &m.Classes[i]
		if m.sideByID[cl.Side] == nil {
			return fmt.Errorf("class %q: unknown side %q", cl.ID, cl.Side)
		}
		for id, v := range cl.Skills {
			if !seenSkill[id] {
				return fmt.Errorf("class %q: unknown skill %q", cl.ID, id)
			}
			if v < 0 || v > 100 {
				return fmt.Errorf("class %q: skill %q must be 0 to 100", cl.ID, id)
			}
		}
		m.classByID[cl.ID] = cl
	}
	for _, et := range m.EdgeTypes {
		for _, c := range et.Crossings {
			if m.Crossing(c) == nil {
				return fmt.Errorf("edge %q: unknown crossing %q", et.ID, c)
			}
		}
	}
	seen := map[string]bool{}
	for i := range m.Events {
		e := &m.Events[i]
		if seen[e.ID] {
			return fmt.Errorf("duplicate event %q", e.ID)
		}
		seen[e.ID] = true
		if err := e.Validate(m); err != nil {
			return fmt.Errorf("event %q: %w", e.ID, err)
		}
	}
	return nil
}
