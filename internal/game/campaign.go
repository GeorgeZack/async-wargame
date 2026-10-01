package game

import (
	"fmt"
	"slices"
	"time"
)

// A room is a campaign of one or more operations. When the referee ends an
// operation, every friendly unit still on the map has survived it: it gains
// an operation of experience and a line in its service record. The next
// operation starts from the scenario template, with survivors in place of
// their template counterparts and green replacements for units that were
// lost. Experience levels and their bonus dice come from the module.

type OperationRecord struct {
	Number    int       `json:"number"`
	Scenario  string    `json:"scenario"`
	Turns     int       `json:"turns"`
	EndedAt   time.Time `json:"endedAt"`
	Survivors []string  `json:"survivors"`
	Lost      []string  `json:"lost"`
	Stats     OpStats   `json:"stats"` // the after-action review
}

// ExperienceOf is the unit's current experience level.
func ExperienceOf(mod *Module, u *Unit) XPLevel { return mod.Experience.Level(u.Operations) }

// EndOperation closes the current operation and sets up the next one.
func (r *Room) EndOperation(actor *Member, sc *Scenario, mod *Module, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase == PhaseSetup {
		return bad("the operation hasn't started yet")
	}
	r.Undo = nil
	op := max(r.Operation, 1)
	rec := OperationRecord{Number: op, Scenario: sc.Name, Turns: r.Turn, EndedAt: now.UTC(), Stats: r.Stats}

	survivors := map[string]*Unit{}
	for _, u := range r.Units {
		if !mod.Side(u.Side).Playable {
			continue
		}
		before := ExperienceOf(mod, u)
		u.Operations++
		after := ExperienceOf(mod, u)
		line := fmt.Sprintf("Operation %d (%s): survived, %d turns.", op, sc.Name, r.Turn)
		if after.Name != before.Name {
			line += " Promoted to " + after.Name + "."
		}
		u.Record = append(u.Record, line)
		survivors[u.ID] = u
		rec.Survivors = append(rec.Survivors, u.Name)
	}
	fresh := newRoom(r.Code, r.Name, sc, mod, "", "")
	var units []*Unit
	for _, tu := range fresh.Units {
		if !mod.Side(tu.Side).Playable {
			units = append(units, tu) // civilians and infected start over
			continue
		}
		if s := survivors[tu.ID]; s != nil {
			s.Col, s.Row = tu.Col, tu.Row // survivors redeploy at the start positions
			units = append(units, s)
			delete(survivors, tu.ID)
			continue
		}
		rec.Lost = append(rec.Lost, tu.Name)
		tu.Name += " (replacements)"
		tu.Record = []string{fmt.Sprintf("Raised as replacements for operation %d.", op+1)}
		units = append(units, tu)
	}
	for _, s := range r.Units { // survivors the referee added during play keep their place
		if survivors[s.ID] == s {
			units = append(units, s)
		}
	}
	r.Units = units
	r.Map, r.Features = fresh.Map, fresh.Features
	r.Operations = append(r.Operations, rec)
	r.Operation = op + 1
	r.Phase, r.Turn, r.Threat = PhaseSetup, 0, MinThreat
	r.Deadline, r.LockedAt, r.Deferred = time.Time{}, time.Time{}, false
	r.Sightings, r.Resolution, r.Stats = nil, nil, OpStats{}
	for _, e := range r.Events {
		if e.Status == EventProposed {
			e.Status, e.DecidedAt = EventDiscarded, &now
		}
	}
	r.Orders = slices.DeleteFunc(r.Orders, func(o *Order) bool { return o.Status == OrderDraft || o.Status == OrderCommitted })
	msg := fmt.Sprintf("Operation %d is over after %d turns. %s survived", op, rec.Turns, plural(len(rec.Survivors), "unit", "units"))
	if len(rec.Lost) > 0 {
		msg += fmt.Sprintf("; %s lost and replaced", joinList(rec.Lost))
	}
	r.logf(VisAll, "%s. Operation %d is being prepared.", msg, op+1)
	return nil
}

// xpNote reminds the referee of a unit's experience dice.
func xpNote(mod *Module, u *Unit) string {
	if l := ExperienceOf(mod, u); l.Dice > 0 {
		return fmt.Sprintf(" (%s: +%d dice)", u.Name+" is "+l.Name, l.Dice)
	}
	return ""
}
