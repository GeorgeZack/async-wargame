package game

import "fmt"

// UnitCard is one unit, as the viewer may see it: who they are, what
// they're good at, what they carry, how they are, and what's been
// established about them.
type UnitCard struct {
	Unit       UnitView
	Counter    Stack // for drawing the counter
	SideName   string
	Sector     string
	Contact    bool // a player's view of an unidentified unit
	Playable   bool
	Quality    int    // the unit's quality bonus
	Veteran    int    // suggested veterancy bonus
	Bonus      string // what a roll usually starts from: "+3 (quality +2, veteran +1)"
	Kit        string
	Notes      string
	Record     []string
	Operations int
	Commander  string
	RadioText  string
	RadioDown  bool
	Order      *OrderView
	Facts      []FactView
	CanPropose bool // the viewer may propose a fact about this unit
	CanEdit    bool // the referee
	RawID      string
	Strength   int
	Status     string
	Known      bool
	Identified bool
	Kits       string // the template's kit, for the referee's edit form
}

// BuildUnitCard finds a unit by id (or, for players, by contact id) and
// returns what me may see of it. During adjudication, players see units as
// they were at the lock.
func BuildUnitCard(r *Room, mod *Module, me *Member, id string) (*UnitCard, error) {
	ref := me.Role == RoleReferee
	units := r.Units
	if !ref && r.Phase == PhaseResolving && r.LockedUnits != nil {
		units = r.LockedUnits
	}
	var u *Unit
	for _, x := range units {
		side := mod.Side(x.Side)
		switch {
		case side == nil:
		case x.ID == id && (ref || side.Playable):
			u = x
		case !ref && !side.Playable && x.Known && contactID(r, x) == id:
			u = x
		}
	}
	if u == nil {
		return nil, ErrNotFound
	}
	side := mod.Side(u.Side)
	uv := unitView(r, u, mod, me)
	card := &UnitCard{Unit: uv, SideName: side.Name, Sector: r.Map.SectorAt(u.Col, u.Row), Contact: uv.Contact,
		Playable: side.Playable, Counter: Stack{Top: uv, Count: 1}}
	if at, stale := r.reportedAt(u, me); stale {
		card.Sector = r.Map.SectorAt(at[0], at[1])
		card.Unit.Hex, card.Unit.Stale = HexID(at[0], at[1]), true
	}
	if uv.Contact {
		card.SideName = "Unknown"
		return card, nil
	}
	card.Quality, card.Veteran, card.Kit, card.Notes = QualityOf(mod, u), VeteranOf(u), KitOf(mod, u), u.Notes
	if b, why := baseBonus(mod, u); why != "" {
		card.Bonus = fmt.Sprintf("%+d (%s)", b, why)
	} else {
		card.Bonus = "+0"
	}
	card.Facts = r.FactsFor(me, u.ID)
	if ref {
		card.CanEdit, card.RawID, card.Strength, card.Status = true, u.ID, u.Strength, u.Status
		card.Known, card.Identified, card.Kits = u.Known, u.Identified, mod.Class(u.Class).Kit
	}
	if side.Playable {
		card.Operations, card.Record = u.Operations, u.Record
		card.CanPropose = me.Status == StatusApproved
		if m := r.Member(u.AssignedTo); m != nil {
			card.Commander = m.Name
		}
		card.RadioText, card.RadioDown = r.radioText(u), u.OutOfContact
		if o := r.CurrentOrder(u.ID); o != nil {
			shared := o.Status == OrderCommitted || o.Status == OrderLocked
			if r.seesOrder(me, u) && (ref || shared || r.CanOrder(mod, me, u)) {
				ov := r.orderView(o, false)
				card.Order = &ov
			}
		}
	}
	return card, nil
}
