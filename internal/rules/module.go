// Package rules holds a game module's schema and the dice. A module is data:
// sides, unit templates, suggested factors, map art, and random events. The
// referee interprets everything else: the dice only say how each side did,
// and the referee (or the AI assistant, with the referee's approval) says
// what that means in the story.
//
// The campaign server (package game) depends on this package, never the
// other way round, so a new setting can be written without touching it.
package rules

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// ErrInvalid marks a request or module that breaks the rules.
var ErrInvalid = errors.New("bad request")

// Module is a versioned setting.
type Module struct {
	Awareness *Awareness `json:"awareness,omitempty"`
	Engine    string     `json:"engine,omitempty"` // empty: wargame; wayward: exploration playtest
	ID        string     `json:"id"`
	Version   string     `json:"version"`
	Name      string     `json:"name"`
	Intro     string     `json:"intro,omitempty"`
	Factors   []Factor   `json:"factors"` // things that often count toward a roll's bonus
	Sides     []Side     `json:"sides"`
	Classes   []Class    `json:"classes"` // unit templates
	Terrain   []Terrain  `json:"terrain"` // map art
	EdgeTypes []EdgeType `json:"edges"`
	Crossings []Crossing `json:"crossings"`
	Events    []EventDef `json:"events"`

	terrainByCode map[byte]*Terrain
	classByID     map[string]*Class
	sideByID      map[string]*Side
}

// Awareness bounds the opposition's observations, in board cells. A scenario
// may override it. Revealed contacts bypass concealment, not range or obstacles.
type Awareness struct {
	RangeCells     float64 `json:"rangeCells"`
	ConcealedCells float64 `json:"concealedCells"`
}

func (a Awareness) Validate() error {
	if math.IsNaN(a.RangeCells) || math.IsInf(a.RangeCells, 0) || a.RangeCells <= 0 || a.RangeCells > 100 ||
		math.IsNaN(a.ConcealedCells) || math.IsInf(a.ConcealedCells, 0) || a.ConcealedCells < 0 || a.ConcealedCells > a.RangeCells {
		return fmt.Errorf("awareness needs rangeCells greater than 0 and at most 100, and concealedCells between 0 and rangeCells")
	}
	return nil
}

// Factor is something that often counts toward one side's bonus: "ambush
// +3", "night -1". A suggestion for the referee, not a rule.
type Factor struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type Side struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Playable bool   `json:"playable"`
	Enemy    bool   `json:"enemy"`
}

// Class is a unit template: what a unit of this kind starts with.
type Class struct {
	Character *WaywardCharacter `json:"character,omitempty"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Abbr      string            `json:"abbr"`
	Side      string            `json:"side"`
	Kind      string            `json:"kind,omitempty"`   // "group" (strength is a headcount) or a single "special", "monster", "nest", "vehicle"
	Symbol    string            `json:"symbol,omitempty"` // counter art
	// Strength is the usual headcount for a new unit of this kind.
	Strength int `json:"strength,omitempty"`
	// Quality is how good a unit of this kind is, as a bonus to its rolls:
	// 0 for ordinary, +1 or +2 for trained, more for the exceptional.
	Quality     int    `json:"quality"`
	Description string `json:"description,omitempty"`
	Kit         string `json:"kit,omitempty"` // what it usually carries, in a sentence
	// Speed is how fast it moves at a careful, tactical pace, and Run flat
	// out, both in km/h. They tell the referee and the AI how far it can
	// realistically get in a turn.
	Speed   float64  `json:"speed,omitempty"`
	Run     float64  `json:"run,omitempty"`
	Contact *Contact `json:"contact,omitempty"` // how players see it before it's identified
}

type Contact struct {
	Abbr  string `json:"abbr"`
	Label string `json:"label"`
}

func (c *Class) IsGroup() bool { return c.Kind == "" || c.Kind == "group" }

// Terrain is how a hex is drawn and what it's called.
type Terrain struct {
	Conceals    bool   `json:"conceals,omitempty"`
	BlocksSight bool   `json:"blocksSight,omitempty"`
	Code        string `json:"code"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsWater     bool   `json:"water,omitempty"`
	Note        string `json:"note,omitempty"`
}

func (t *Terrain) Water() bool { return t.IsWater }

// EdgeType is a feature drawn along hex sides: a river, a ridge, a wall.
type EdgeType struct {
	BlocksSight bool     `json:"blocksSight,omitempty"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Blocks      bool     `json:"blocks,omitempty"` // a real obstacle; random events route around it
	Crossings   []string `json:"crossings"`        // crossings allowed on this edge
	Note        string   `json:"note,omitempty"`
}

type Crossing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

func (m *Module) TerrainAt(code byte) *Terrain { return m.terrainByCode[code] }
func (m *Module) Class(id string) *Class       { return m.classByID[id] }
func (m *Module) Side(id string) *Side         { return m.sideByID[id] }

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
	if m.Awareness != nil {
		if err := m.Awareness.Validate(); err != nil {
			return err
		}
	}
	if m.Engine != "" && m.Engine != "wayward" {
		return fmt.Errorf("unknown rules engine %q", m.Engine)
	}
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
	m.classByID = map[string]*Class{}
	for i := range m.Classes {
		cl := &m.Classes[i]
		if m.sideByID[cl.Side] == nil {
			return fmt.Errorf("class %q: unknown side %q", cl.ID, cl.Side)
		}
		if cl.Quality < -5 || cl.Quality > 10 {
			return fmt.Errorf("class %q: quality must be -5 to 10", cl.ID)
		}
		if m.Engine == "wayward" {
			if cl.Character == nil || cl.Strength != 1 || cl.IsGroup() || !m.sideByID[cl.Side].Playable || m.sideByID[cl.Side].Enemy {
				return fmt.Errorf("Wayward requires individual characters")
			}
			if err := cl.Character.Validate(); err != nil {
				return err
			}
		} else if cl.Character != nil {
			return fmt.Errorf("Wayward character sheets require the Wayward engine")
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
