package game

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// Publishing a turn makes its resolution canonical: units move, fights take
// effect, civilians board dropships, units resupply, and everyone gets a
// report. Then the next turn opens.

// TurnReport is the situation report for one turn. Players and referee get
// different versions: players never see what the infected really are.
type TurnReport struct {
	Operation int       `json:"operation"`
	Turn      int       `json:"turn"`
	At        time.Time `json:"at"`
	Summary   string    `json:"summary"`
	Players   []string  `json:"players"`
	Referee   []string  `json:"referee"`

	Replays      []*Replay   `json:"replays,omitempty"`
	PlayerLinks  map[int]int `json:"playerLinks,omitempty"` // line index → replay ID
	RefereeLinks map[int]int `json:"refereeLinks,omitempty"`
}

// OpStats are the running totals for the after-action review.
type OpStats struct {
	CiviliansStart     int      `json:"civiliansStart"`
	Evacuated          int      `json:"evacuated"`
	CiviliansLost      int      `json:"civiliansLost"`
	FriendlyCasualties int      `json:"friendlyCasualties"`
	FriendlyLost       []string `json:"friendlyLost,omitempty"`
	EnemyKilled        int      `json:"enemyKilled"`
	EnemyDestroyed     int      `json:"enemyDestroyed"`
	Encounters         int      `json:"encounters"`
	Won                int      `json:"won"`
	Lost               int      `json:"lost"`
	Turns              int      `json:"turns"`
}

// civilianTotal counts neutral people on the map, for the evacuation score.
func (r *Room) civilianTotal(mod *Module) int {
	n := 0
	for _, u := range r.Units {
		if roleOf(mod, u) == "neutral" {
			n += u.Strength
		}
	}
	return n
}

// Unsettled counts encounters the referee hasn't settled yet.
func (r *Room) Unsettled() int {
	if r.Resolution == nil || r.Resolution.Published || r.Resolution.Turn != r.Turn {
		return 0
	}
	n := 0
	for _, e := range r.Resolution.Encounters {
		if !e.Settled {
			n++
		}
	}
	return n
}

func (r *Room) encounter(id string) *Encounter {
	if r.Resolution == nil {
		return nil
	}
	for _, e := range r.Resolution.Encounters {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// SettleEncounter is the referee's ruling on one fight. mode is "rules"
// (roll the suggested pools), "roll" (roll the given pools), or "hits"
// (use hits rolled at the table).
func (r *Room) SettleEncounter(actor *Member, mod *Module, id, mode string, dice, hits [2]int, reason string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhaseResolving {
		return bad("there's no turn being resolved")
	}
	e := r.encounter(id)
	if e == nil {
		return ErrNotFound
	}
	rng, _ := newRNG()
	switch mode {
	case "rules":
		r.settle(mod, e, rng, nil, nil, reason)
	case "roll":
		if dice[0] < 0 || dice[1] < 0 || dice[0] > 60 || dice[1] > 60 {
			return bad("pools must be 0 to 60 dice")
		}
		r.settle(mod, e, rng, &dice, nil, reason)
	case "hits":
		if hits[0] < 0 || hits[1] < 0 || hits[0] > dice[0] || hits[1] > dice[1] {
			return bad("hits must be between 0 and the number of dice")
		}
		r.settle(mod, e, rng, &dice, &hits, reason)
	default:
		return bad("unknown way to settle")
	}
	if e.Source == "referee" && strings.TrimSpace(e.Reason) == "" {
		e.Reason = "Referee ruling"
	}
	return nil
}

// SettleAll lets the rule engine roll every unsettled encounter.
func (r *Room) SettleAll(actor *Member, mod *Module) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	r.settleRemaining(mod)
	return nil
}

func (r *Room) settleRemaining(mod *Module) {
	if r.Resolution == nil {
		return
	}
	rng := rngFor(r.Resolution.Seed + 1)
	for _, e := range r.Resolution.Encounters {
		if !e.Settled {
			r.settle(mod, e, rng, nil, nil, "")
		}
	}
}

// publish applies the resolution and opens the next turn.
func (r *Room) publish(mod *Module, now time.Time) {
	res := r.Resolution
	rep := &TurnReport{Operation: max(r.Operation, 1), Turn: r.Turn, At: now.UTC()}
	if res == nil || res.Turn != r.Turn {
		res = &Resolution{Turn: r.Turn}
	}
	for _, u := range r.Units {
		u.Suppressed = false
	}
	addBoth := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
	}

	// 1. Movement.
	for _, m := range res.Moves {
		u := r.Unit(m.Unit)
		if u == nil || u.Col != m.From()[0] || u.Row != m.From()[1] {
			continue
		}
		to := m.To()
		u.Col, u.Row = to[0], to[1]
		line := fmt.Sprintf("%s moved %s → %s (%s)%s.", u.Name, HexID(m.From()[0], m.From()[1]), HexID(to[0], to[1]),
			r.Map.SectorAt(to[0], to[1]), map[string]string{"engaged": ", stopped by contact", "stacking": ", short of a full hex", "stopped": ", short of its goal"}[m.Reason])
		switch roleOf(mod, u) {
		case "player", "neutral":
			addBoth("%s", line)
			id := rep.addReplay(r.moveReplay(mod, u, m, false))
			rep.linkPlayers(id)
			rep.linkReferee(id)
		default:
			rep.Referee = append(rep.Referee, line)
			rep.linkReferee(rep.addReplay(r.moveReplay(mod, u, m, true)))
		}
	}
	for _, n := range res.Notes {
		addBoth("%s", n)
	}

	// Getting off, riding along, and climbing aboard.
	for _, o := range r.Orders {
		if o.Turn != r.Turn || o.Status != OrderLocked {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil {
			continue
		}
		if t := mod.Orders.Task(o.Task); t != nil && t.Disembark && u.Aboard != "" {
			from := r.Unit(u.Aboard)
			u.Aboard = ""
			if o.HasDest && len(o.Route) == 0 { // off under the transport
				u.Col, u.Row = o.DestCol, o.DestRow
			}
			if from != nil {
				addBoth("%s got off %s at %s.", u.Name, from.Name, HexID(u.Col, u.Row))
			}
		}
	}
	r.syncPassengers()
	for _, o := range r.Orders {
		if o.Turn != r.Turn || o.Status != OrderLocked || o.Transport == "" {
			continue
		}
		u, t := r.Unit(o.Unit), r.Unit(o.Transport)
		if u == nil {
			continue
		}
		if t == nil {
			addBoth("%s couldn't board: its transport is gone.", u.Name)
			continue
		}
		if err := r.board(mod, u, t); err != nil {
			addBoth("%s couldn't board %s: %s.", u.Name, t.Name, err)
		} else {
			addBoth("%s boarded %s.", u.Name, t.Name)
		}
	}

	// 2. Fights.
	won, lost := 0, 0
	for _, e := range res.Encounters {
		if !e.Settled {
			continue
		}
		r.applyEncounter(mod, e, rep, res)
		r.Stats.Encounters++
		switch {
		case e.Winner == "friend":
			won++
		case e.Winner == "enemy":
			lost++
		}
	}
	r.Stats.Won += won
	r.Stats.Lost += lost

	// 3. Resupply and 4. evacuation.
	r.resupply(mod, rep)
	r.evacuate(mod, rep)

	r.Stats.Turns = r.Turn
	parts := []string{plural(len(res.Encounters), "fight", "fights")}
	if len(rep.Players) == 0 {
		rep.Players = []string{"A quiet hour. Nothing changed hands."}
	}
	if r.Stats.CiviliansStart > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d civilians evacuated so far", r.Stats.Evacuated, r.Stats.CiviliansStart))
	}
	rep.Summary = fmt.Sprintf("Turn %d: %s.", r.Turn, strings.Join(parts, "; "))
	r.Reports = append(r.Reports, rep)
	res.Published = true
	r.logKind(VisAll, "sitrep", "%s The full turn report is on the Reports screen.", rep.Summary)
	r.openTurn(r.Turn+1, now)
	r.carryOver(mod, now)
}

// applyEncounter makes a settled fight's outcome happen.
func (r *Room) applyEncounter(mod *Module, e *Encounter, rep *TurnReport, res *Resolution) {
	paths := map[string][][2]int{}
	for _, m := range res.Moves {
		paths[m.Unit] = m.Path
	}
	refReplay, plReplay, finish := r.fightReplays(mod, e, paths)
	defer finish()
	friends := slices.Concat(e.Friend, e.Neutral)
	where := fmt.Sprintf("%s (%s)", HexID(e.Col, e.Row), r.Map.SectorAt(e.Col, e.Row))
	names := func(ids []string) string {
		var n []string
		for _, id := range ids {
			if u := r.Unit(id); u != nil {
				n = append(n, u.Name)
			}
		}
		return joinList(n)
	}
	o := e.Outcome
	var winners, losers []string
	switch e.Winner {
	case "friend":
		winners, losers = friends, e.Enemy
	case "enemy":
		winners, losers = e.Enemy, friends
	}
	rep.Referee = append(rep.Referee, fmt.Sprintf("Fight at %s, slice %d: %s (%d dice, %d hits) against %s (%d dice, %d hits). %s %s",
		where, e.Slice, names(friends), e.Dice[0], e.Hits[0], names(e.Enemy), e.Dice[1], e.Hits[1], winnerText(e), o.Result))
	rep.linkReferee(rep.addReplay(refReplay))
	// Players hear about a fight only if one of their units was in it, or
	// civilians they already know about.
	known := len(e.Friend) > 0 || slices.ContainsFunc(e.Neutral, func(id string) bool {
		u := r.Unit(id)
		return u != nil && r.VisibleToPlayers(mod, u)
	})
	tell := func(line string) {
		if known {
			rep.Players = append(rep.Players, line)
		}
	}
	ours := names(e.Friend)
	if ours == "" {
		ours = names(e.Neutral)
	}
	tell(fmt.Sprintf("Fight at %s: %s against %s. %s %s", where, ours,
		plural(len(e.Enemy), "contact", "contacts"), playerWinnerText(e), o.Result))
	if known {
		rep.linkPlayers(rep.addReplay(plReplay))
	}

	// Ammunition for every friendly unit in the fight.
	for _, id := range e.Friend {
		if u := r.Unit(id); u != nil && mod.Combat.Ammo > 0 {
			cl := mod.Class(u.Class)
			if u.Supply == nil {
				u.Supply = map[string]int{}
			}
			for k := range cl.Ammo {
				u.Supply[k] = max(0, u.ammo(cl, k)-mod.Combat.Ammo)
			}
			if outOfAmmo(cl, u) {
				rep.Players = append(rep.Players, fmt.Sprintf("%s is out of ammunition and needs to resupply.", u.Name))
			}
		}
	}

	victimsLost := 0
	hurt := func(ids []string, pct int, loser bool) {
		for _, id := range ids {
			u := r.Unit(id)
			if u == nil || pct <= 0 {
				continue
			}
			// Groups lose people; a single creature or vehicle only goes down
			// when the result says all of it.
			n := min(u.Strength, int(math.Ceil(float64(u.Strength)*float64(pct)/100)))
			if !mod.Class(u.Class).IsGroup() && roleOf(mod, u) == "enemy" {
				n = u.Strength * pct / 100
			}
			if n == 0 {
				continue
			}
			u.Strength -= n
			role := roleOf(mod, u)
			switch role {
			case "player":
				r.Stats.FriendlyCasualties += n
			case "enemy":
				r.Stats.EnemyKilled += n
			default:
				r.Stats.CiviliansLost += n
				if inf := mod.Combat.Infection; inf != nil && u.Side == inf.Victims {
					victimsLost += n
				}
			}
			if u.Strength <= 0 {
				r.destroy(mod, u, rep, where, known)
				continue
			}
			if role != "enemy" {
				tell(fmt.Sprintf("%s lost %d.", u.Name, n))
			}
			rep.Referee = append(rep.Referee, fmt.Sprintf("%s lost %d (%d left).", u.Name, n, u.Strength))
			if loser && o.Loser.Suppressed {
				u.Suppressed = true
			}
		}
	}
	hurt(losers, o.Loser.Casualties, true)
	hurt(winners, o.Winner.Casualties, false)

	// The infected turn the dead.
	if inf := mod.Combat.Infection; inf != nil && victimsLost > 0 && e.Winner == "enemy" {
		var biggest *Unit
		for _, id := range e.Enemy {
			if u := r.Unit(id); u != nil && u.Side == inf.Side && mod.Class(u.Class).IsGroup() && (biggest == nil || u.Strength > biggest.Strength) {
				biggest = u
			}
		}
		if gain := int(math.Round(float64(victimsLost) * inf.Rate)); biggest != nil && gain > 0 {
			biggest.Strength += gain
			rep.Referee = append(rep.Referee, fmt.Sprintf("%s grows by %d as the dead rise.", biggest.Name, gain))
		}
	}

	// Falling back.
	for _, id := range losers {
		if u := r.Unit(id); u != nil && o.Loser.Retreat > 0 {
			r.retreat(mod, u, e, o.Loser.Retreat)
		}
	}
}

func winnerText(e *Encounter) string {
	switch e.Winner {
	case "friend":
		return fmt.Sprintf("Friendly side wins by %d.", e.Margin)
	case "enemy":
		return fmt.Sprintf("The infected win by %d.", e.Margin)
	}
	return "Even."
}

func playerWinnerText(e *Encounter) string {
	switch e.Winner {
	case "friend":
		return "We came out on top."
	case "enemy":
		return "We were driven back."
	}
	return "Neither side gave ground."
}

// destroy removes a unit wiped out in a fight.
func (r *Room) destroy(mod *Module, u *Unit, rep *TurnReport, where string, known bool) {
	r.Units = slices.DeleteFunc(r.Units, func(x *Unit) bool { return x == u })
	// Passengers of a lost transport are set down, hurt, or lost with it at sea.
	for _, p := range r.passengers(u) {
		p.Aboard = ""
		if mod.TerrainAt(r.Map.TerrainCode(u.Col, u.Row)).Water() {
			r.destroy(mod, p, rep, where, known)
			continue
		}
		lost := (p.Strength + 3) / 4
		p.Strength -= lost
		line := fmt.Sprintf("%s escaped the wreck of %s, losing %d.", p.Name, u.Name, lost)
		rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
	}
	switch roleOf(mod, u) {
	case "player":
		r.Stats.FriendlyLost = append(r.Stats.FriendlyLost, u.Name)
		line := fmt.Sprintf("%s was lost at %s.", u.Name, where)
		rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
	case "enemy":
		r.Stats.EnemyDestroyed++
		if known {
			rep.Players = append(rep.Players, fmt.Sprintf("A contact at %s was destroyed.", where))
		}
		rep.Referee = append(rep.Referee, fmt.Sprintf("%s was destroyed at %s.", u.Name, where))
	default:
		line := fmt.Sprintf("%s were overrun at %s.", u.Name, where)
		if known {
			rep.Players = append(rep.Players, line)
		}
		rep.Referee = append(rep.Referee, line)
	}
}

// retreat moves a unit up to n hexes away from the foes it fought.
func (r *Room) retreat(mod *Module, u *Unit, e *Encounter, n int) {
	cl := mod.Class(u.Class)
	var foes [][2]int
	for _, id := range slices.Concat(e.Friend, e.Neutral, e.Enemy) {
		if f := r.Unit(id); f != nil && opposed(mod, u, f) {
			foes = append(foes, [2]int{f.Col, f.Row})
		}
	}
	for range n {
		_, here := nearest(u.Col, u.Row, foes)
		best, bestD := [2]int{-1, -1}, here
		load := r.Load(mod)
		load[[2]int{u.Col, u.Row}] -= stackPoints(mod, u)
		for dir := range 6 {
			if r.stepCost(mod, cl, u.Col, u.Row, dir) < 0 {
				continue
			}
			nc, nr, _ := r.Map.Neighbor(u.Col, u.Row, dir)
			h := [2]int{nc, nr}
			if _, d := nearest(nc, nr, foes); d > bestD && r.fits(mod, u, h, load) {
				best, bestD = h, d
			}
		}
		if best[0] < 0 {
			return
		}
		u.Col, u.Row = best[0], best[1]
	}
}

// resupply refills units ordered to resupply at a source or carrier.
func (r *Room) resupply(mod *Module, rep *TurnReport) {
	for _, u := range r.Units {
		o := r.CurrentOrder(u.ID)
		if roleOf(mod, u) != "player" || o == nil || o.Status != OrderLocked || mod.Orders.Task(o.Task) == nil || !mod.Orders.Task(o.Task).Resupply {
			continue
		}
		ok := slices.ContainsFunc(r.Features, func(f Feature) bool {
			return slices.Contains(mod.Supply.Sources, f.Kind) && HexDistance(f.Col, f.Row, u.Col, u.Row) <= 1
		}) || slices.ContainsFunc(r.Units, func(c *Unit) bool {
			return c != u && slices.Contains(mod.Supply.Carriers, c.Class) && HexDistance(c.Col, c.Row, u.Col, u.Row) <= 1
		})
		if ok {
			u.Supply = nil // full
			line := fmt.Sprintf("%s resupplied.", u.Name)
			rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
		} else {
			line := fmt.Sprintf("%s couldn't resupply: no supply point or transport within reach.", u.Name)
			rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
		}
	}
}

// evacuate boards civilians on an evacuation beach onto carriers offshore.
func (r *Room) evacuate(mod *Module, rep *TurnReport) {
	ev := mod.Evacuation
	if ev.CarrierClass == "" {
		return
	}
	for _, carrier := range slices.Clone(r.Units) {
		if carrier.Class != ev.CarrierClass {
			continue
		}
		room := mod.Class(carrier.Class).Capacity
		for _, c := range slices.Clone(r.Units) {
			if room <= 0 || roleOf(mod, c) != "neutral" || HexDistance(c.Col, c.Row, carrier.Col, carrier.Row) != 1 ||
				!slices.Contains(ev.Terrain, mod.TerrainAt(r.Map.TerrainCode(c.Col, c.Row)).ID) {
				continue
			}
			n := min(room, c.Strength)
			room -= n
			c.Strength -= n
			r.Stats.Evacuated += n
			line := fmt.Sprintf("%d civilians from %s boarded %s at %s.", n, c.Name, carrier.Name, HexID(c.Col, c.Row))
			rep.Players, rep.Referee = append(rep.Players, line), append(rep.Referee, line)
			if c.Strength <= 0 {
				r.Units = slices.DeleteFunc(r.Units, func(x *Unit) bool { return x == c })
			}
		}
	}
}

// PublishTurn is the referee approving the resolution and opening the next turn.
func (r *Room) PublishTurn(actor *Member, mod *Module, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhaseResolving {
		return bad("the turn hasn't been locked yet")
	}
	if n := r.Unsettled(); n > 0 {
		return bad("settle the %s first, or let the rules settle them", plural(n, "remaining fight", "remaining fights"))
	}
	r.publish(mod, now)
	return nil
}

// newResolutionRNG is used by tests to make resolution repeatable.
var newResolutionRNG = func() (*rand.Rand, uint64) { return newRNG() }
