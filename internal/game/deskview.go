package game

import "slices"

// DeskView is the referee's adjudication desk.
type DeskView struct {
	Items      []DeskItem    // one per locked order
	Loose      []ContestView // rolls not tied to an order
	Extra      []*Outcome    // outcomes not tied to an order
	Unwritten  int
	Rolled     int
	Unrolled   int
	Units      []UnitOption // who can roll
	Skills     []Skill
	Advantages []Advantage
	Missing    []string // player units with no order this turn
}

type DeskItem struct {
	Order    OrderView
	Unit     UnitView
	Skills   []SkillLine
	Kit      string
	Contests []ContestView
	Outcome  *Outcome
	Busy     bool // the assistant is drafting its outcome
}

type ContestView struct {
	*Contest
	Summary string
	Text    string
}

type UnitOption struct {
	ID, Name, SideName string
	Enemy              bool
}

func contestView(c *Contest) ContestView {
	cv := ContestView{Contest: c, Summary: ResultSummary(c)}
	if c.Result != nil {
		cv.Text = c.Result.Text()
	}
	return cv
}

func buildDesk(r *Room, mod *Module, me *Member, ai AIView) *DeskView {
	if r.Phase != PhaseResolving || r.Desk == nil {
		return nil
	}
	d := r.Desk
	dv := &DeskView{Skills: mod.Skills, Advantages: mod.Advantages}
	for _, s := range mod.Sides {
		for _, u := range r.Units {
			if u.Side == s.ID {
				dv.Units = append(dv.Units, UnitOption{ID: u.ID, Name: u.Name, SideName: s.Name, Enemy: s.Enemy})
			}
		}
	}
	ordered := map[string]bool{}
	for _, o := range r.LockedOrders() {
		u := r.Unit(o.Unit)
		if u == nil {
			continue
		}
		ordered[u.ID] = true
		it := DeskItem{Order: r.orderView(o, false), Unit: unitView(r, u, mod, me), Skills: SkillLines(mod, u), Kit: KitOf(mod, u),
			Outcome: d.outcomeFor(o.ID), Busy: ai.Busy[RunOutcome+"/"+o.ID]}
		for _, c := range d.Contests {
			if c.Order == o.ID {
				it.Contests = append(it.Contests, contestView(c))
			}
		}
		if it.Outcome == nil || it.Outcome.Text == "" {
			dv.Unwritten++
		}
		dv.Items = append(dv.Items, it)
	}
	for _, c := range d.Contests {
		if c.Result != nil {
			dv.Rolled++
		} else {
			dv.Unrolled++
		}
		if c.Order == "" {
			dv.Loose = append(dv.Loose, contestView(c))
		}
	}
	for _, oc := range d.Outcomes {
		if oc.Order == "" {
			dv.Extra = append(dv.Extra, oc)
		}
	}
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable && !ordered[u.ID] && r.memberOrNil(u.AssignedTo) != nil {
			dv.Missing = append(dv.Missing, u.Name)
		}
	}
	return dv
}

// ReportView is a published report as one member may read it.
type ReportView struct {
	Operation, Turn int
	Corrected       string
	Narrative       string // players: only once approved
	NarrativeStatus string
	NarrativeModel  string
	NarrativeEdited bool
	Latest          bool
	Entries         []EntryView
}

type EntryView struct {
	ReportEntry
	Mine   bool
	Hidden bool // referee: players don't see this entry
}

func addReports(v *View, r *Room, me *Member) {
	ref := me.Role == RoleReferee
	for i := len(r.Reports) - 1; i >= 0 && len(v.Reports) < 10; i-- {
		rep := r.Reports[i]
		rv := ReportView{Operation: rep.Operation, Turn: rep.Turn, Corrected: rep.Corrected, Latest: i == len(r.Reports)-1}
		if ref || rep.NarrativeStatus == NarrativeApproved {
			rv.Narrative, rv.NarrativeStatus, rv.NarrativeModel, rv.NarrativeEdited = rep.Narrative, rep.NarrativeStatus, rep.NarrativeModel, rep.NarrativeEdited
		}
		for _, e := range rep.Entries {
			if !r.seesEntry(me, e) {
				continue
			}
			ev := EntryView{ReportEntry: e, Mine: e.Holder == me.ID, Hidden: e.Seen != SeenByAll}
			if !ref {
				ev.Secret = nil
			}
			ev.Rolls = slices.Clone(e.Rolls)
			rv.Entries = append(rv.Entries, ev)
		}
		v.Reports = append(v.Reports, rv)
	}
	v.AARs = slices.Clone(r.Operations)
	slices.Reverse(v.AARs)
}
