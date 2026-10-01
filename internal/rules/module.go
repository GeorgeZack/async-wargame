// Package rules holds a game module's schema and the engine that interprets
// it. A module is data: terrain, sides, unit classes, orders, combat, and
// events, as JSON. The engine knows a fixed vocabulary of operations and
// conditions; the module supplies the numbers and words. The campaign server
// (package game) depends on this package, never the other way round, so a
// new ruleset can be written without touching the server.
package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalid marks a request or module that breaks the rules.
var ErrInvalid = errors.New("bad request")

func bad(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Module is a versioned mechanics module: terrain, sides, and unit classes.
// Resolution rules will be added here as the declarative rule format settles.
type Module struct {
	ID         string     `json:"id"`
	Version    string     `json:"version"`
	Name       string     `json:"name"`
	EngineAPI  string     `json:"engineApi"`
	Profile    string     `json:"profile"`
	Scale      Scale      `json:"scale"`
	Sides      []Side     `json:"sides"`
	Terrain    []Terrain  `json:"terrain"`
	Classes    []Class    `json:"classes"`
	Traits     []Trait    `json:"traits"`
	EdgeTypes  []EdgeType `json:"edges"`
	Orders     OrderSpec  `json:"orders"`
	Stacking   Stacking   `json:"stacking"`
	Combat     Combat     `json:"combat"`
	Sight      Sight      `json:"sight"`
	Evacuation Evacuation `json:"evacuation"`
	Supply     Supply     `json:"supply"`
	Escort     Escort     `json:"escort"`
	Comms      *Comms     `json:"comms,omitempty"` // nil: everyone is always in contact
	Experience Experience `json:"experience"`
	Crossings  []Crossing `json:"crossings"`
	Events     []EventDef `json:"events"`

	terrainByCode map[byte]*Terrain
	traitByID     map[string]*Trait
	classByID     map[string]*Class
	sideByID      map[string]*Side
}

type Scale struct {
	Hex         string `json:"hex"`
	Turn        string `json:"turn"`
	Slices      int    `json:"slices"`
	Provisional bool   `json:"provisional"`
}

type Side struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Playable bool   `json:"playable"`
	Enemy    bool   `json:"enemy"`
}

type Terrain struct {
	Code  string `json:"code"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Move  int    `json:"move"` // movement cost; 0 means impassable to ground units
	Cover int    `json:"cover"`
	Stack int    `json:"stack"` // friendly stacking points a hex holds
	Note  string `json:"note,omitempty"`
}

func (t *Terrain) Water() bool { return t.Move == 0 }

type Class struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Abbr     string         `json:"abbr"`
	Side     string         `json:"side"`
	Domain   string         `json:"domain"` // "land" or "water"
	Movement int            `json:"movement"`
	Attack   int            `json:"attack"`
	Range    int            `json:"range"`
	Armor    int            `json:"armor"`
	Capacity int            `json:"capacity,omitempty"`
	Ammo     map[string]int `json:"ammo,omitempty"`
	// Kind is "group" (strength is a headcount), or "special", "monster", or
	// "nest" (a single entity). Only groups get strength estimates as contacts.
	Kind    string      `json:"kind,omitempty"`
	Symbol  string      `json:"symbol,omitempty"`  // counter symbol drawn by the client's map style
	Stack   int         `json:"stack,omitempty"`   // stacking points one unit uses; 0 = not counted
	AI      *AIDoctrine `json:"ai,omitempty"`      // enemy AI behavior
	Embarks bool        `json:"embarks,omitempty"` // can ride a transport (one with Capacity), counted by strength
	Traits  []string    `json:"traits,omitempty"`
	Contact *Contact    `json:"contact,omitempty"` // how the unit appears to the other side before identification
}

type Contact struct {
	Abbr  string `json:"abbr"`
	Label string `json:"label"`
}

func (c *Class) IsGroup() bool { return c.Kind == "" || c.Kind == "group" }

// EdgeType is a linear feature along hex sides. Blocks means ground units
// cannot cross it without a crossing; Cost is extra movement to cross.
type EdgeType struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Blocks    bool     `json:"blocks"`
	Cost      int      `json:"cost"`
	Crossings []string `json:"crossings"` // crossing ids allowed on this edge
	Note      string   `json:"note,omitempty"`
}

type Crossing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
	Note string `json:"note,omitempty"`
}

// Experience turns operations survived into a level and bonus dice.
type Experience struct {
	Note   string    `json:"note"`
	Levels []XPLevel `json:"levels"`
}

type XPLevel struct {
	Name string `json:"name"`
	Min  int    `json:"min"` // operations survived
	Dice int    `json:"dice"`
	Note string `json:"note"`
}

// Level is the experience level for a number of operations survived.
func (e Experience) Level(ops int) XPLevel {
	var lvl XPLevel
	for _, l := range e.Levels {
		if ops >= l.Min {
			lvl = l
		}
	}
	return lvl
}

// Stacking limits how many friendly units fit in a hex, in points.
type Stacking struct {
	Default int    `json:"default"`
	Note    string `json:"note,omitempty"`
}

// OrderSpec lists the order fields a module offers.
type OrderSpec struct {
	Tasks     []TaskDef `json:"tasks"`
	Risks     []Option  `json:"risks"`
	Fallbacks []Option  `json:"fallbacks"`
}

type TaskDef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Note        string `json:"note,omitempty"`
	Destination bool   `json:"destination,omitempty"` // needs a destination hex
	Offensive   bool   `json:"offensive,omitempty"`
	Careful     bool   `json:"careful,omitempty"`   // stops short of contact, whatever the risk
	Resupply    bool   `json:"resupply,omitempty"`  // refills ammunition at a supply point
	Embark      bool   `json:"embark,omitempty"`    // board a transport
	Disembark   bool   `json:"disembark,omitempty"` // get off a transport
}

type Option struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	StopsShort bool   `json:"stopsShort,omitempty"` // risk: stop before coming into contact
	Default    bool   `json:"default,omitempty"`
}

func (o OrderSpec) Task(id string) *TaskDef {
	for i := range o.Tasks {
		if o.Tasks[i].ID == id {
			return &o.Tasks[i]
		}
	}
	return nil
}

func OptionName(opts []Option, id string) string {
	for _, o := range opts {
		if o.ID == id {
			return o.Name
		}
	}
	return ""
}

type Trait struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
}

func (m *Module) TerrainAt(code byte) *Terrain { return m.terrainByCode[code] }
func (m *Module) Class(id string) *Class       { return m.classByID[id] }
func (m *Module) Side(id string) *Side         { return m.sideByID[id] }
func (m *Module) Trait(id string) *Trait       { return m.traitByID[id] }

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
	for _, t := range m.Terrain {
		if t.ID == id {
			return true
		}
	}
	return false
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
	m.classByID = map[string]*Class{}
	for i := range m.Classes {
		cl := &m.Classes[i]
		if m.sideByID[cl.Side] == nil {
			return fmt.Errorf("class %q: unknown side %q", cl.ID, cl.Side)
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
	m.traitByID = map[string]*Trait{}
	for i := range m.Traits {
		m.traitByID[m.Traits[i].ID] = &m.Traits[i]
	}
	for _, cl := range m.Classes {
		for _, t := range cl.Traits {
			if m.traitByID[t] == nil {
				return fmt.Errorf("class %q: unknown trait %q", cl.ID, t)
			}
		}
	}
	if err := m.Combat.Validate(); err != nil {
		return err
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

// Sight sets how far friendly units see and how cover hides things.
type Sight struct {
	Base            int            `json:"base"`
	Elevated        int            `json:"elevated"`        // from ElevatedTerrain
	ElevatedTerrain []string       `json:"elevatedTerrain"` // terrain ids
	Water           int            `json:"water"`           // for water units (sensors)
	Cover           []CoverRange   `json:"cover"`           // the most cover that applies wins
	Traits          map[string]int `json:"traits"`          // trait id -> the farthest it can be seen
	Note            string         `json:"note,omitempty"`
}

type CoverRange struct {
	Cover int `json:"cover"`
	Range int `json:"range"` // a unit in this much cover is seen only this close
}

// Range is how far a unit of class cl standing on terrain t sees.
func (s Sight) RangeFor(cl *Class, t *Terrain) int {
	if cl.Domain == "water" && s.Water > 0 {
		return s.Water
	}
	if slices.Contains(s.ElevatedTerrain, t.ID) && s.Elevated > 0 {
		return s.Elevated
	}
	return max(1, s.Base)
}

// Concealment caps how far away a unit of class cl on terrain t can be seen.
func (s Sight) Concealment(cl *Class, t *Terrain) int {
	limit := 99
	for _, tr := range cl.Traits {
		if n, ok := s.Traits[tr]; ok {
			limit = min(limit, n)
		}
	}
	best := -1
	for _, c := range s.Cover {
		if t.Cover >= c.Cover && c.Cover > best {
			best = c.Cover
			limit = min(limit, c.Range)
		}
	}
	return limit
}

// AIDoctrine is how the enemy AI moves a class. Behaviors the engine knows:
// "hunt" (go for the nearest prey it senses), "ambush" (wait until prey is
// within sense range, then strike), "call" (stand still and draw nearby units
// that answer calls toward its prey), and "hold" (never move).
type AIDoctrine struct {
	Sense       int    `json:"sense"`
	Behavior    string `json:"behavior"`
	Prefers     string `json:"prefers,omitempty"` // "isolated": prefer lone playable units
	Wanders     bool   `json:"wanders,omitempty"` // shuffles about when it senses nothing
	AnswersCall bool   `json:"answersCall,omitempty"`
	CallRange   int    `json:"callRange,omitempty"`
}

// Evacuation: neutral units on these terrains next to water holding a
// carrier class leave the map, up to the carrier's capacity each turn.
type Evacuation struct {
	Terrain      []string `json:"terrain"`
	CarrierClass string   `json:"carrierClass"`
	Note         string   `json:"note,omitempty"`
}

// Supply: a Resupply order refills ammunition at or next to these feature
// kinds, or alongside these classes.
type Supply struct {
	Sources  []string `json:"sources"`
	Carriers []string `json:"carriers"`
	Note     string   `json:"note,omitempty"`
}

// Escort: neutral units in the same hex as a unit with one of these tasks
// move with it.
type Escort struct {
	Tasks []string `json:"tasks"`
	Note  string   `json:"note,omitempty"`
}

// Comms is the command net. A playable unit is in contact if it's within a
// node's range, or within Relay hexes of another unit in contact. Nodes are
// features (Manned: only while a friendly unit is within one hex) or units of
// a class.
type Comms struct {
	Relay int         `json:"relay"`
	Nodes []CommsNode `json:"nodes"`
	Note  string      `json:"note,omitempty"`
}

type CommsNode struct {
	Feature string `json:"feature,omitempty"` // feature kind
	Class   string `json:"class,omitempty"`   // unit class
	Range   int    `json:"range"`
	Manned  bool   `json:"manned,omitempty"`
}
