package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// AutoBatches checkpoints one simultaneous turn. Each unit and order has one
// writer. Every batch sees the same authoritative positions and all other orders;
// decisions are merged and validated before anything is published.
type AutoBatches struct {
	Groups    [][]string     `json:"groups"`
	Index     int            `json:"index"`
	Input     string         `json:"input"`
	Decisions []AutoDecision `json:"decisions,omitempty"`
}

const autoBatchSize = 8
const autoBatchThreshold = 12

func (r *Room) prepareAutoBatches() {
	if r.Wayward != nil || r.Assistant.Batches != nil || len(r.Units) <= autoBatchThreshold {
		return
	}
	units := slices.Clone(r.Units)
	sort.SliceStable(units, func(i, j int) bool {
		bandI, bandJ := int(units[i].Y/(r.Board.Cell*8)), int(units[j].Y/(r.Board.Cell*8))
		if bandI != bandJ {
			return bandI < bandJ
		}
		if units[i].X != units[j].X {
			return units[i].X < units[j].X
		}
		return units[i].Y < units[j].Y
	})
	p := &AutoBatches{Input: r.autoFingerprint()}
	used := map[string]bool{}
	for _, u := range units {
		if used[u.ID] {
			continue
		}
		group := []string{u.ID}
		used[u.ID] = true
		if u.Formation != "" {
			for _, v := range units {
				if v.Formation == u.Formation && !used[v.ID] {
					group = append(group, v.ID)
					used[v.ID] = true
				}
			}
		}
		n := len(p.Groups)
		if n == 0 || len(p.Groups[n-1])+len(group) > autoBatchSize {
			p.Groups = append(p.Groups, group)
		} else {
			p.Groups[n-1] = append(p.Groups[n-1], group...)
		}
	}
	r.Assistant.Batches = p
}
func (r *Room) autoOwnsUnit(id string) bool {
	p := r.Assistant.Batches
	return p == nil || p.Index >= 0 && p.Index < len(p.Groups) && slices.Contains(p.Groups[p.Index], id)
}
func (r *Room) autoOwnsOrder(id string) bool {
	o := r.lockedOrder(id)
	return o != nil && r.autoOwnsUnit(o.Unit)
}
func (r *Room) autoBatchOrders() []*Order {
	var out []*Order
	for _, o := range r.LockedOrders() {
		if r.autoOwnsOrder(o.ID) {
			out = append(out, o)
		}
	}
	return out
}
func (r *Room) nextAutoBatch(kind string) (string, string) {
	p := r.Assistant.Batches
	next := map[string]string{RunAutoIntentions: RunAutoContests, RunAutoContests: RunAutoResolve, RunAutoResolve: RunAutoPublish}[kind]
	if p == nil {
		return next, ""
	}
	p.Index++
	if p.Index < len(p.Groups) {
		next = kind
	} else {
		p.Index = 0
	}
	p.Input = r.autoFingerprint()
	if next == RunAutoPublish {
		return next, ""
	}
	return next, fmt.Sprintf("batch-%d", p.Index+1)
}
func (r *Room) mergeAutoBatches(mod *Module) error {
	p := r.Assistant.Batches
	d := &AutoDecision{}
	var reasons []string
	for i, part := range p.Decisions {
		d.Outcomes = append(d.Outcomes, part.Outcomes...)
		d.Units = append(d.Units, part.Units...)
		d.Model = part.Model
		if part.Threat != nil && (d.Threat == nil || *part.Threat > *d.Threat) {
			v := *part.Threat
			d.Threat = &v
		}
		reasons = append(reasons, fmt.Sprintf("Batch %d: %s", i+1, clip(part.Reason, 180)))
	}
	d.Reason = clip(strings.Join(reasons, "\n"), 3000)
	test := r.cloneAuto()
	if err := test.applyAutoDecision(mod, d); err != nil {
		return err
	}
	d.Input = r.autoFingerprint()
	r.Assistant.Decision = d
	return nil
}

// AutomaticReply avoids spending a provider request on empty enemy batches.
func (r *Room) AutomaticReply(mod *Module, run *Run) (string, bool) {
	if r.Wayward != nil && run.Kind == RunAutoResolve && r.Desk != nil {
		d := AutoDecision{Reason: "Server applied the frozen Wayward rulings, destinations and dice."}
		for _, a := range r.Desk.Wayward {
			if r.Unit(a.Unit) == nil {
				return `{"reason":"A ruled character no longer exists; pause and review the turn."}`, true
			}
			seen := SeenByAll
			if a.Secret {
				seen = SeenByReferee
			}
			d.Outcomes = append(d.Outcomes, AutoOutcome{Order: a.Order, Title: "Action", Text: a.Consequence(), Seen: seen})
			if a.Succeeded() && a.At != nil && *a.At != r.Unit(a.Unit).Pos() {
				d.Units = append(d.Units, AutoUnitEdit{Unit: a.Unit, At: a.At})
			}
		}
		b, _ := json.Marshal(d)
		return string(b), true
	}
	if run.Kind != RunAutoIntentions {
		return "", false
	}
	for _, u := range r.Units {
		if r.autoOwnsUnit(u.ID) && !u.Destroyed() && mod.Side(u.Side).Enemy {
			return "", false
		}
	}
	return `{"intentions":[]}`, true
}

// Batch-specific detail plus a compact whole-turn context prevents duplicate
// ownership without pretending distant orders or cross-batch combat don't exist.
func (r *Room) autoBatchContext(mod *Module) any {
	if r.Assistant.Batches == nil {
		return nil
	}
	type contextUnit struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Side    string `json:"side"`
		At      Pos    `json:"at"`
		Count   int    `json:"count"`
		Status  string `json:"status,omitempty"`
		Intends string `json:"intends,omitempty"`
		Hidden  bool   `json:"hiddenFromPlayers,omitempty"`
	}
	var units []contextUnit
	var orders []map[string]any
	for _, u := range r.Units {
		if !r.autoOwnsUnit(u.ID) {
			units = append(units, contextUnit{u.ID, u.Name, u.Side, u.Pos(), u.Strength, u.Status, r.IntentionOf(u.ID), !r.PubliclyKnown(mod, u)})
		}
	}
	for _, o := range r.LockedOrders() {
		if r.autoOwnsOrder(o.ID) {
			continue
		}
		var members []string
		for _, u := range r.OrderMembers(o) {
			members = append(members, u.ID)
		}
		orders = append(orders, map[string]any{"id": o.ID, "text": o.Text, "members": members, "mark": o.Mark})
	}
	return map[string]any{"units": units, "orders": orders}
}

// Keep prior prose out of large prompts: current state and recorded dice remain
// authoritative; past reports are a bounded aid, not a second world snapshot.
func (r *Room) autoRecentReports() any {
	reports := r.Reports[max(0, len(r.Reports)-2):]
	if r.Assistant.Batches == nil {
		return reports
	}
	var out []map[string]any
	for _, rep := range reports {
		out = append(out, map[string]any{"turn": rep.Turn, "narrative": rep.Narrative})
	}
	return out
}

func (r *Room) validateBatchDecision(mod *Module, d *AutoDecision) error {
	for _, u := range d.Units {
		if !r.autoOwnsUnit(u.Unit) {
			return bad("batch cannot change another batch's unit %s", u.Unit)
		}
	}
	for _, o := range d.Outcomes {
		if o.Order != "" && !r.autoOwnsOrder(o.Order) {
			var required []string
			for _, own := range r.autoBatchOrders() {
				required = append(required, own.ID)
			}
			return bad("batch cannot resolve order %s; return outcomes for these IDs only: %s", o.Order, strings.Join(required, ", "))
		}
	}
	// Validate using the ordinary full-turn validator, supplying unchanged outcomes
	// only for orders owned by other batches. These placeholders are never stored.
	b, _ := json.Marshal(d)
	var check AutoDecision
	_ = json.Unmarshal(b, &check)
	for _, o := range r.LockedOrders() {
		if !r.autoOwnsOrder(o.ID) {
			check.Outcomes = append(check.Outcomes, AutoOutcome{Order: o.ID, Title: "Pending", Text: "Pending another batch.", Seen: SeenByReferee})
		}
	}
	return r.cloneAuto().applyAutoDecision(mod, &check)
}

// A batch needs dice for its own orders and any contests involving its units.
// Foreign contests stay on the canonical desk and reach their owning batches.
func (r *Room) autoBatchDesk() *Desk {
	if r.Assistant.Batches == nil || r.Desk == nil {
		return r.Desk
	}
	d := &Desk{Turn: r.Desk.Turn}
	for _, c := range r.Desk.Contests {
		if r.autoOwnsOrder(c.Order) || r.autoOwnsUnit(c.AUnit) || c.BUnit != "" && r.autoOwnsUnit(c.BUnit) {
			d.Contests = append(d.Contests, c)
		}
	}
	for _, o := range r.Desk.Outcomes {
		if o.Order != "" && r.autoOwnsOrder(o.Order) {
			d.Outcomes = append(d.Outcomes, o)
		}
	}
	return d
}

func (r *Room) publicDecisionFacts(mod *Module, after *Room) []string {
	var facts []string
	viewer := &Member{ID: "public-briefing", Role: RolePlayer, Status: StatusApproved}
	for _, u := range after.Units {
		old := r.Unit(u.ID)
		if old == nil || u.OutOfContact || old.OutOfContact || !after.PubliclyKnown(mod, u) {
			continue
		}
		shown := unitView(after, u, mod, viewer)
		if !r.PubliclyKnown(mod, old) {
			facts = append(facts, fmt.Sprintf("Recorded new public contact: %s at %s.", shown.Name, shown.Where))
		} else if old.Pos() != u.Pos() {
			facts = append(facts, fmt.Sprintf("Recorded movement: %s moved from %s to %s.", shown.Name, r.Where(old.Pos(), false), shown.Where))
		}
		if u.Strength < old.Strength {
			if u.Destroyed() {
				facts = append(facts, fmt.Sprintf("Recorded loss: %s is destroyed.", shown.Name))
			} else if mod.Side(u.Side).Playable {
				facts = append(facts, fmt.Sprintf("Recorded casualties: %s strength %d to %d.", shown.Name, old.Strength, u.Strength))
			} else {
				facts = append(facts, fmt.Sprintf("Recorded casualties: %s took losses.", shown.Name))
			}
		}
	}
	return facts
}

// Retry smaller units of work after an invalid resolution. Completed proposals
// and every server roll stay intact; whole formations still have one writer.
func (r *Room) splitFailedAutoBatch() bool {
	p := r.Assistant.Batches
	if p == nil || p.Index >= len(p.Groups) || len(p.Groups[p.Index]) < 2 {
		return false
	}
	source := p.Groups[p.Index]
	limit := (len(source) + 1) / 2
	used := map[string]bool{}
	var groups [][]string
	for _, id := range source {
		if used[id] {
			continue
		}
		u := r.Unit(id)
		group := []string{id}
		used[id] = true
		if u != nil && u.Formation != "" {
			for _, other := range source {
				v := r.Unit(other)
				if !used[other] && v != nil && v.Formation == u.Formation {
					group = append(group, other)
					used[other] = true
				}
			}
		}
		n := len(groups)
		if n == 0 || len(groups[n-1])+len(group) > limit {
			groups = append(groups, group)
		} else {
			groups[n-1] = append(groups[n-1], group...)
		}
	}
	if len(groups) < 2 {
		return false
	}
	combined := append([][]string{}, p.Groups[:p.Index]...)
	combined = append(combined, groups...)
	combined = append(combined, p.Groups[p.Index+1:]...)
	p.Groups = combined
	return true
}
