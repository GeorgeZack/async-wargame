package game

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
	Skills     []SkillLine
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
	card.Skills, card.Kit, card.Notes = SkillLines(mod, u), KitOf(mod, u), u.Notes
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

// Rules is everything the rules page prints, drawn from the module.
type Rules struct {
	Module      *Module
	Scale       Scale
	Schedule    string
	MinPlanning string
	Templates   []RuleClass // the player side's unit templates
	Others      []RuleClass // referee only
	IsReferee   bool
}

type RuleClass struct {
	*Class
	SideName string
	Skills   []SkillLine
}

// BuildRules prepares the rules page.
func BuildRules(r *Room, mod *Module, me *Member) *Rules {
	ref := me.Role == RoleReferee
	rl := &Rules{Module: mod, Scale: mod.Scale, IsReferee: ref, MinPlanning: "an hour",
		Schedule: "Orders lock at " + joinTimes(r.Schedule) + ", " + daysText(r.Schedule.Days) + "."}
	for i := range mod.Classes {
		cl := &mod.Classes[i]
		side := mod.Side(cl.Side)
		rc := RuleClass{Class: cl, SideName: side.Name, Skills: SkillLines(mod, &Unit{Class: cl.ID})}
		switch {
		case side.Playable:
			rl.Templates = append(rl.Templates, rc)
		case ref:
			rl.Others = append(rl.Others, rc)
		}
	}
	return rl
}
