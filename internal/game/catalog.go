package game

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

// Scenario defines a map, features, starting forces, and briefing text.
type Scenario struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Module       string            `json:"module"`
	Summary      string            `json:"summary"`
	Briefing     []string          `json:"briefing"`
	RefereeNotes string            `json:"refereeNotes"`
	Map          HexMap            `json:"map"`
	Board        Board             `json:"board"`
	Features     []ScenarioFeature `json:"features"`
	Units        []ScenarioUnit    `json:"units"`
}

// Spot is where something starts: a drawn hex (col, row, from 0), or an
// exact point (x, y) on the board.
type Spot struct {
	Col int      `json:"col"`
	Row int      `json:"row"`
	X   *float64 `json:"x,omitempty"`
	Y   *float64 `json:"y,omitempty"`
}

func (s Spot) Pos() Pos {
	if s.X != nil && s.Y != nil {
		return Pos{*s.X, *s.Y}
	}
	return center(s.Col, s.Row)
}

type ScenarioUnit struct {
	ID string `json:"id"`
	Spot
	Name     string `json:"name"`
	Class    string `json:"class"`
	Strength int    `json:"strength"`
	Known    bool   `json:"known"`
	Brief    string `json:"brief,omitempty"` // private, for whoever plays it
}

type ScenarioFeature struct {
	ID string `json:"id"`
	Spot
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Hidden bool   `json:"hidden,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Catalog holds every module and scenario available to new campaigns.
type Catalog struct {
	Modules   map[string]*Module
	Scenarios map[string]*Scenario
}

func (c *Catalog) ScenarioList() []*Scenario {
	out := make([]*Scenario, 0, len(c.Scenarios))
	for _, s := range c.Scenarios {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LoadCatalog reads modules/*.json and scenarios/*.json from each source
// in turn and validates them. Later sources may add modules and scenarios
// (and scenarios may use earlier sources' modules); an id that's already
// loaded is an error, so a mod can't silently replace a built-in ruleset.
func LoadCatalog(sources ...fs.FS) (*Catalog, error) {
	c := &Catalog{Modules: map[string]*Module{}, Scenarios: map[string]*Scenario{}}
	for _, fsys := range sources {
		mods, err := fs.Glob(fsys, "modules/*.json")
		if err != nil {
			return nil, err
		}
		for _, p := range mods {
			var m Module
			if err := readJSON(fsys, p, &m); err != nil {
				return nil, err
			}
			if err := m.Index(); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if c.Modules[m.ID] != nil {
				return nil, fmt.Errorf("%s: a module with id %q is already loaded", p, m.ID)
			}
			c.Modules[m.ID] = &m
		}
	}
	for _, fsys := range sources {
		scens, err := fs.Glob(fsys, "scenarios/*.json")
		if err != nil {
			return nil, err
		}
		for _, p := range scens {
			var s Scenario
			if err := readJSON(fsys, p, &s); err != nil {
				return nil, err
			}
			m := c.Modules[s.Module]
			if m == nil {
				return nil, fmt.Errorf("%s: unknown module %q", p, s.Module)
			}
			if err := s.validate(m); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if c.Scenarios[s.ID] != nil {
				return nil, fmt.Errorf("%s: a scenario with id %q is already loaded", p, s.ID)
			}
			c.Scenarios[s.ID] = &s
		}
	}
	return c, nil
}

func readJSON(fsys fs.FS, p string, v any) error {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path.Base(p), err)
	}
	return nil
}

func (s *Scenario) validate(m *Module) error {
	if err := s.Map.validate(m); err != nil {
		return err
	}
	for _, f := range s.Features {
		if f.X == nil && !s.Map.InBounds(f.Col, f.Row) {
			return fmt.Errorf("feature %q is off the map", f.ID)
		}
	}
	seen := map[string]bool{}
	for _, u := range s.Units {
		if seen[u.ID] {
			return fmt.Errorf("duplicate unit id %q", u.ID)
		}
		seen[u.ID] = true
		if m.Class(u.Class) == nil {
			return fmt.Errorf("unit %q: unknown class %q", u.ID, u.Class)
		}
		if u.X == nil && !s.Map.InBounds(u.Col, u.Row) {
			return fmt.Errorf("unit %q is off the map", u.ID)
		}
	}
	return nil
}

// HexID is the conventional four-digit wargame hex number, 1-based: column then row.
func HexID(col, row int) string { return fmt.Sprintf("%02d%02d", col+1, row+1) }
