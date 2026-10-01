package game

import (
	"fmt"
	"slices"
	"sort"
)

// UnitCard is the full stat card for one unit, as the viewer may see it.
type UnitCard struct {
	Unit       UnitView
	Counter    Stack // for drawing the counter
	SideName   string
	Sector     string
	Contact    bool // a player's view of an enemy: signature only
	Class      *Class
	StackPts   int
	Traits     []Trait
	Ammo       []string
	Level      XPLevel
	NextLevel  *XPLevel
	Operations int
	Record     []string
	Commander  string
	RadioText  string
	RadioDown  bool
	Order      *OrderView
	Levels     []XPLevel
}

// BuildUnitCard finds a unit by id (or, for players, by contact id) and
// returns what me may see of it.
func BuildUnitCard(r *Room, mod *Module, me *Member, id string) (*UnitCard, error) {
	ref := me.Role == RoleReferee
	var u *Unit
	for _, x := range r.Units {
		side := mod.Side(x.Side)
		switch {
		case x.ID == id && (ref || !side.Enemy) && (ref || r.VisibleToPlayers(mod, x)):
			u = x
		case !ref && side.Enemy && contactID(r, x) == id:
			u = x
		}
	}
	if u == nil {
		return nil, ErrNotFound
	}
	side := mod.Side(u.Side)
	cl := mod.Class(u.Class)
	card := &UnitCard{SideName: side.Name, Levels: mod.Experience.Levels}
	if !ref && side.Enemy {
		// Players get a contact card: where, and what it looks like. No identity.
		if r.VisibleToPlayers(mod, u) {
			card.Unit = unitView(u, mod, false)
			card.Sector = r.Map.SectorAt(u.Col, u.Row)
		} else if sg, ok := r.Sightings[u.ID]; ok {
			card.Unit = ghostView(r, u, mod)
			card.Sector = r.Map.SectorAt(sg.Col, sg.Row)
		} else {
			return nil, ErrNotFound
		}
		card.Unit.ID, card.Contact, card.SideName = id, true, "Unknown"
		card.Counter = Stack{Top: card.Unit, Count: 1}
		return card, nil
	}
	card.Unit = unitView(u, mod, ref)
	card.Counter = Stack{Top: card.Unit, Count: 1}
	card.Sector = r.Map.SectorAt(u.Col, u.Row)
	card.Class = cl
	card.StackPts = stackPoints(mod, u)
	for _, t := range cl.Traits {
		if tr := mod.Trait(t); tr != nil {
			card.Traits = append(card.Traits, *tr)
		}
	}
	keys := make([]string, 0, len(cl.Ammo))
	for k := range cl.Ammo {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		card.Ammo = append(card.Ammo, fmt.Sprintf("%s ×%d", k, cl.Ammo[k]))
	}
	if side.Playable {
		card.Level, card.Operations, card.Record = ExperienceOf(mod, u), u.Operations, u.Record
		if i := slices.IndexFunc(mod.Experience.Levels, func(l XPLevel) bool { return l.Min > u.Operations }); i >= 0 {
			next := mod.Experience.Levels[i]
			card.NextLevel = &next
		}
		if m := r.Member(u.AssignedTo); m != nil {
			card.Commander = m.Name
		}
		if r.CommsActive(mod) || u.OutOfContact {
			card.RadioText, card.RadioDown = r.radioText(u), u.OutOfContact
		}
		if o := r.CurrentOrder(u.ID); o != nil {
			shared := o.Status == OrderCommitted || o.Status == OrderLocked
			if r.seesOrder(me, u) && (ref || shared || r.CanOrder(mod, me, u)) {
				ov := r.orderView(mod, o, false)
				card.Order = &ov
			}
		}
	}
	return card, nil
}

// Rules is everything the rules page prints, drawn from the module so the
// page always matches what the server enforces.
type Rules struct {
	Module      *Module
	Scale       Scale
	Schedule    string
	Takeover    string
	Classes     []RuleClass
	Infected    []RuleClass // referee only
	Traits      []Trait     // referee only
	Terrain     []RuleTerrain
	SightBase   int
	SightHigh   int
	MinPlanning string
	IsReferee   bool
	Operation   int
}

type RuleClass struct {
	*Class
	SideName string
	Traits   []string
}

type RuleTerrain struct {
	Terrain
	Capacity int
}

func BuildRules(r *Room, mod *Module, me *Member) *Rules {
	ref := me.Role == RoleReferee
	rules := &Rules{Module: mod, Scale: mod.Scale, IsReferee: ref, SightBase: mod.Sight.Base, SightHigh: mod.Sight.Elevated,
		Schedule: fmt.Sprintf("Orders lock at %s, %s.", joinTimes(r.Schedule), daysText(r.Schedule.Days)),
		Takeover: TakeoverLabel(r.Schedule.Takeover), MinPlanning: "an hour", Operation: max(r.Operation, 1)}
	for i := range mod.Classes {
		cl := &mod.Classes[i]
		side := mod.Side(cl.Side)
		rc := RuleClass{Class: cl, SideName: side.Name}
		for _, t := range cl.Traits {
			if tr := mod.Trait(t); tr != nil {
				rc.Traits = append(rc.Traits, tr.Name)
			}
		}
		if side.Enemy {
			if ref {
				rules.Infected = append(rules.Infected, rc)
			}
			continue
		}
		rules.Classes = append(rules.Classes, rc)
	}
	if ref {
		rules.Traits = mod.Traits
	}
	for _, t := range mod.Terrain {
		limit := t.Stack
		if limit == 0 && !t.Water() {
			limit = mod.Stacking.Default
		}
		rules.Terrain = append(rules.Terrain, RuleTerrain{Terrain: t, Capacity: limit})
	}
	return rules
}
