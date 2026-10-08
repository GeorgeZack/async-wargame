package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// engine is the rules a room plays under. Room code asks the engine instead
// of testing which engine's state happens to be present, so a new ruleset is
// one more implementation here rather than another branch in every file.
//
// The d10 wargame is the default; a module opts into another with "engine".
type engine interface {
	// id is the module's "engine" value ("" for the wargame).
	id() string

	// setup prepares a new room's engine state from its scenario.
	setup(r *Room, sc *Scenario)
	// rosterError says why another unit can't be added in the editor, if it can't.
	rosterError(r *Room, strength int) error
	// startError says why the operation can't start with this roster, if it can't.
	startError(r *Room) error
	// requiresModel reports whether play is impossible without a text model.
	requiresModel() bool
	// botProfile overrides the playtest bots' personality; "" keeps their own.
	botProfile() string

	// d10Contests reports whether the referee's opposed d10 rolls, roll
	// suggestions and drafted outcomes apply.
	d10Contests() bool
	// editableSheets reports whether the referee edits units from their cards
	// and players propose facts about them.
	editableSheets() bool
	// batches reports whether a large automatic turn is split into ownership batches.
	batches() bool
	// allReady reports whether the turn should lock before its deadline
	// because everyone has acted.
	allReady(r *Room) bool
	// discovered reports whether a discovery objective has been met.
	discovered(r *Room, discovery string) bool
	// discoveries is what the party has established, for the player page.
	discoveries(r *Room) []string

	// settingText describes the game and its rolls to the model.
	settingText(r *Room, mod *Module) string
	// promptVersion is recorded on each run so results can be compared later.
	promptVersion(run *Run) string
	// autoPrompt replaces the automatic contest and resolution prompts; handled
	// is false when the engine uses the standard ones.
	autoPrompt(r *Room, mod *Module, run *Run) (system, user string, ok, handled bool)
	// applyContests replaces the automatic contest stage; handled is false
	// when the engine uses d10 contests.
	applyContests(r *Room, mod *Module, reply string, decode func(string, any) error) (handled bool, err error)
	// localReply answers an automatic stage on the server, without the model.
	localReply(r *Room, run *Run) (string, bool)
	// validateDecision adds the engine's own checks on an automatic decision.
	validateDecision(r *Room, d *AutoDecision) error
	// decisionApplied records engine state once a decision's changes are made.
	decisionApplied(r *Room)
}

const engineWayward = "wayward"

// rules is the engine this room plays under.
func (r *Room) rules() engine {
	if r.Wayward != nil {
		return waywardEngine{}
	}
	return wargameEngine{}
}

func engineFor(mod *Module) engine {
	if mod.Engine == engineWayward {
		return waywardEngine{}
	}
	return wargameEngine{}
}

// RequiresAI reports whether this room's rules can't be played without a
// configured text model.
func (r *Room) RequiresAI() bool { return r.rules().requiresModel() }

// BotProfile is how playtest bots should play this room's rules; "" leaves
// each bot its own personality.
func (r *Room) BotProfile() string { return r.rules().botProfile() }

// --- the d10 wargame ---

type wargameEngine struct{}

func (wargameEngine) id() string                            { return "" }
func (wargameEngine) setup(*Room, *Scenario)                {}
func (wargameEngine) rosterError(*Room, int) error          { return nil }
func (wargameEngine) startError(*Room) error                { return nil }
func (wargameEngine) requiresModel() bool                   { return false }
func (wargameEngine) botProfile() string                    { return "" }
func (wargameEngine) d10Contests() bool                     { return true }
func (wargameEngine) editableSheets() bool                  { return true }
func (wargameEngine) batches() bool                         { return true }
func (wargameEngine) allReady(*Room) bool                   { return false }
func (wargameEngine) discovered(*Room, string) bool         { return false }
func (wargameEngine) discoveries(*Room) []string            { return nil }
func (wargameEngine) promptVersion(run *Run) string         { return promptVersions[run.Kind] }
func (wargameEngine) localReply(*Room, *Run) (string, bool) { return "", false }
func (wargameEngine) validateDecision(*Room, *AutoDecision) error {
	return nil
}
func (wargameEngine) decisionApplied(*Room) {}

func (wargameEngine) autoPrompt(*Room, *Module, *Run) (string, string, bool, bool) {
	return "", "", false, false
}

func (wargameEngine) applyContests(*Room, *Module, string, func(string, any) error) (bool, error) {
	return false, nil
}

func (wargameEngine) settingText(r *Room, mod *Module) string {
	var fs []string
	for _, f := range mod.Factors {
		fs = append(fs, fmt.Sprintf("%s %+d", f.Name, f.Value))
	}
	return fmt.Sprintf(`The game: %s. %s %s.
Rolls: when a situation is in doubt, each side rolls a d10 and adds one bonus: everything that matters summed into a single number (how good the unit is, veterancy, the situation, surprise, the weather). The result is just the two totals, like "Okafor 5, the horde 9". There's no rule for what totals mean: the referee compares them, considers what each side was trying to do, and decides. A side can also roll alone, against the situation.
Each unit's usual bonus (quality plus veterancy) is given. Factors that often count: %s.
Movement: each unit's "movePerTurn" says how far it can realistically get in one turn at a careful pace and at a run, at this map's scale. An order's "wantsToReach" says where its pin is and how far that is. Hold orders to that: a unit asked to go farther than it can only gets part of the way, less through rubble, crowds, water, darkness, or a fight, and running is loud and tiring. Say where it ends up.`, mod.Name, mod.Intro, r.scaleForAI(), strings.Join(fs, ", "))
}

// --- Wayward exploration ---

type waywardEngine struct{}

const waywardMaxCharacters = 6

func (waywardEngine) id() string { return engineWayward }

func (waywardEngine) setup(r *Room, sc *Scenario) {
	r.Wayward = newWayward(sc.Challenges)
	r.Assistant.On, r.Assistant.Automatic = true, true
}

func (waywardEngine) rosterError(r *Room, strength int) error {
	if len(r.Units) >= waywardMaxCharacters || strength > 1 {
		return bad("Wayward supports up to six individual characters")
	}
	return nil
}

func (waywardEngine) startError(r *Room) error {
	if len(r.Units) < 1 || len(r.Units) > waywardMaxCharacters || len(r.Formations) > 0 {
		return bad("Wayward needs 1–6 individual characters without formations")
	}
	return nil
}

func (waywardEngine) requiresModel() bool { return true }
func (waywardEngine) botProfile() string {
	return "Explore cooperatively using your character’s strengths. Talk, investigate and follow discovered clues. Change approach after failure; avoid combat and resource spending."
}
func (waywardEngine) d10Contests() bool    { return false }
func (waywardEngine) editableSheets() bool { return false }
func (waywardEngine) batches() bool        { return false }

// Every character must have a current committed action, including host-held
// characters. Missing players wait for the normal deadline.
func (waywardEngine) allReady(r *Room) bool {
	if !r.Assistant.Automatic || !r.Assistant.On || r.Phase != PhasePlanning {
		return false
	}
	n := 0
	for _, u := range r.Units {
		if u.Destroyed() {
			continue
		}
		n++
		o := r.CurrentOrder(u.ID)
		if o == nil || o.Status != OrderCommitted {
			return false
		}
	}
	return n > 0
}

func (waywardEngine) discovered(r *Room, discovery string) bool {
	return slices.Contains(r.Wayward.Discoveries, discovery)
}

func (waywardEngine) discoveries(r *Room) []string {
	var out []string
	for _, id := range r.Wayward.Discoveries {
		if c := r.Wayward.challenge(id); c != nil {
			out = append(out, fmt.Sprintf("%s: %s", c.Name, c.Success))
		}
	}
	return out
}

func (waywardEngine) settingText(_ *Room, mod *Module) string {
	return waywardInstructions + "\n" + mod.Intro
}

func (waywardEngine) promptVersion(run *Run) string {
	if run.Automatic() {
		return "wayward/6/" + promptVersions[run.Kind]
	}
	return promptVersions[run.Kind]
}

func (waywardEngine) autoPrompt(r *Room, mod *Module, run *Run) (string, string, bool, bool) {
	system, user, ok := r.waywardPrompt(mod, run)
	return system, user, ok, true
}

func (waywardEngine) applyContests(r *Room, mod *Module, reply string, decode func(string, any) error) (bool, error) {
	return true, r.applyWaywardActions(mod, reply, decode)
}

// The rulings, destinations and dice were frozen at the contest stage, so the
// resolution is built here rather than asked of the model a second time.
func (waywardEngine) localReply(r *Room, run *Run) (string, bool) {
	if run.Kind != RunAutoResolve || r.Desk == nil {
		return "", false
	}
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

func (waywardEngine) validateDecision(r *Room, d *AutoDecision) error {
	return r.validateWaywardDecision(d)
}

func (waywardEngine) decisionApplied(r *Room) { r.applyWaywardDiscoveries() }
