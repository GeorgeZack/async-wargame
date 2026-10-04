package game

import (
	"fmt"
	"math"
	"slices"
)

// MissionRule is a public, scenario-defined condition, evaluated by the server
// after publication. The AI cannot award completion by claiming it in prose.
// hold: one of Units must hold an uncontested area every turn in the window.
// extract: every named unit must have survivors in the area by the last turn.
type MissionRule struct {
	Discovery string   `json:"discovery,omitempty"`
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Kind      string   `json:"kind"`
	Units     []string `json:"units"`
	Feature   string   `json:"feature"`
	Radius    float64  `json:"radius"` // grid cells
	From      int      `json:"from"`
	Through   int      `json:"through"`
}
type MissionObjective struct {
	Rule        MissionRule `json:"rule"`
	Status      string      `json:"status"` // pending, complete, failed
	CheckedTurn int         `json:"checkedTurn,omitempty"`
}
type MissionState struct {
	Status     string             `json:"status"` // active, success, failure
	EndedTurn  int                `json:"endedTurn,omitempty"`
	Objectives []MissionObjective `json:"objectives"`
}

func newMission(rules []MissionRule) *MissionState {
	if len(rules) == 0 {
		return nil
	}
	m := &MissionState{Status: "active"}
	for _, r := range rules {
		r.Units = slices.Clone(r.Units)
		m.Objectives = append(m.Objectives, MissionObjective{Rule: r, Status: "pending"})
	}
	return m
}
func (r *Room) MissionComplete() bool { return r.Mission != nil && r.Mission.Status != "active" }
func (sc *Scenario) validateMission(mod *Module) error {
	seen := map[string]bool{}
	for _, rule := range sc.Mission {
		if rule.Kind == "discovery" {
			if mod.Engine != "wayward" || rule.ID == "" || seen[rule.ID] || rule.Text == "" || !slices.ContainsFunc(sc.Challenges, func(c WaywardChallenge) bool { return c.ID == rule.Discovery }) {
				return bad("invalid discovery objective")
			}
			seen[rule.ID] = true
			continue
		}
		if rule.ID == "" || seen[rule.ID] || rule.Text == "" || (rule.Kind != "hold" && rule.Kind != "extract") || len(rule.Units) == 0 || rule.From < 1 || rule.Through < rule.From || rule.Through > 100 || rule.Radius < 0 || rule.Radius > 20 || math.IsNaN(rule.Radius) || math.IsInf(rule.Radius, 0) {
			return fmt.Errorf("invalid mission objective %q", rule.ID)
		}
		seen[rule.ID] = true
		if !slices.ContainsFunc(sc.Features, func(f ScenarioFeature) bool { return f.ID == rule.Feature && !f.Hidden }) {
			return fmt.Errorf("objective %q needs a public feature", rule.ID)
		}
		ids := map[string]bool{}
		for _, id := range rule.Units {
			if ids[id] || !slices.ContainsFunc(sc.Units, func(u ScenarioUnit) bool { return u.ID == id && mod.Side(mod.Class(u.Class).Side).Playable }) {
				return fmt.Errorf("objective %q has an invalid player unit", rule.ID)
			}
			ids[id] = true
		}
	}
	return nil
}
func (r *Room) evaluateMission(mod *Module, rep *TurnReport) {
	if r.Mission == nil || r.MissionComplete() {
		return
	}
	alive := false
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable && !u.Destroyed() {
			alive = true
		}
	}
	for i := range r.Mission.Objectives {
		o := &r.Mission.Objectives[i]
		rule := o.Rule
		if o.Status != "pending" {
			continue
		}
		if !alive {
			o.Status = "failed"
			o.CheckedTurn = r.Turn
			continue
		}
		if rule.Kind == "discovery" {
			o.CheckedTurn = r.Turn
			if r.Wayward != nil && slices.Contains(r.Wayward.Discoveries, rule.Discovery) {
				o.Status = "complete"
			}
			continue
		}
		if r.Turn < rule.From {
			continue
		}
		o.CheckedTurn = r.Turn
		var feature *Feature
		for i := range r.Features {
			if r.Features[i].ID == rule.Feature {
				feature = &r.Features[i]
				break
			}
		}
		in := 0
		if feature != nil {
			for _, id := range rule.Units {
				u := r.Unit(id)
				if u != nil && !u.Destroyed() && r.Cells(u.Pos(), feature.Pos()) <= rule.Radius+0.001 {
					in++
				}
			}
		}
		switch rule.Kind {
		case "hold":
			contested := false
			if feature != nil {
				for _, u := range r.Units {
					if mod.Side(u.Side).Enemy && !u.Destroyed() && r.Cells(u.Pos(), feature.Pos()) <= rule.Radius+0.001 {
						contested = true
						break
					}
				}
			}
			if in == 0 || contested {
				o.Status = "failed"
			} else if r.Turn >= rule.Through {
				o.Status = "complete"
			}
		case "extract":
			if in == len(rule.Units) {
				o.Status = "complete"
			} else if r.Turn >= rule.Through {
				o.Status = "failed"
			}
		}
	}
	pending, failed := false, false
	for _, o := range r.Mission.Objectives {
		pending = pending || o.Status == "pending"
		failed = failed || o.Status == "failed"
	}
	if pending {
		return
	}
	r.Mission.Status = "success"
	if failed {
		r.Mission.Status = "failure"
	}
	r.Mission.EndedTurn = r.Turn
	message := fmt.Sprintf("Mission %s at the end of turn %d. The operation is complete; review the objective results before starting another operation.", r.Mission.Status, r.Turn)
	rep.Entries = append(rep.Entries, ReportEntry{Title: "Mission result", Text: message, Seen: SeenByAll})
	r.logf(VisAll, "%s", message)
	r.notifyAll("", "Mission "+r.Mission.Status, message, "reports")
}
