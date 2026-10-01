package game

import (
	"fmt"
	"slices"
)

// Replays. Each report line about a move or a fight can link to a replay: a
// zoomed slice of the map that plays the move back, or for a fight the
// approach, the contact, both dice pools, and the result. A fight has two
// replays: the referee's, with everything, and the players', where the enemy
// is only what was seen at contact.
//
// Replays are only ever played from the Reports screen, when asked for. The
// map itself never animates.

// Replay is one recorded action.
type Replay struct {
	ID      int          `json:"id"`
	Referee bool         `json:"referee,omitempty"` // the referee's version only
	Kind    string       `json:"kind"`              // move or fight
	Title   string       `json:"title"`
	Hex     [2]int       `json:"hex"` // where it ended, or where the fight was
	Units   []ReplayUnit `json:"units"`
	Fight   *ReplayFight `json:"fight,omitempty"`
}

// ReplayUnit is one counter in a replay.
type ReplayUnit struct {
	Name    string   `json:"name"`
	Abbr    string   `json:"abbr"`
	Side    string   `json:"side"`
	Symbol  string   `json:"symbol,omitempty"`
	Contact bool     `json:"contact,omitempty"`
	Enemy   bool     `json:"enemy,omitempty"`
	Path    [][2]int `json:"path"`          // start first; the last hex is where it stood at contact
	End     *[2]int  `json:"end,omitempty"` // where it fell back to after the fight
	Before  int      `json:"before,omitempty"`
	After   int      `json:"after,omitempty"`
	Lost    bool     `json:"lost,omitempty"`
}

// ReplayFight is the dice and the verdict.
type ReplayFight struct {
	Sides   [2]string `json:"sides"`
	Dice    [2]int    `json:"dice"`
	Faces   [2][]int  `json:"faces"`
	Hits    [2]int    `json:"hits"`
	Success int       `json:"success"`
	Winner  string    `json:"winner"`
	Verdict string    `json:"verdict"`
	Result  string    `json:"result"`
}

func (rep *TurnReport) addReplay(rp *Replay) int {
	rp.ID = len(rep.Replays) + 1
	rep.Replays = append(rep.Replays, rp)
	return rp.ID
}

// linkPlayers and linkReferee tie the last line of each version to a replay.
func (rep *TurnReport) linkPlayers(id int) {
	if id > 0 && len(rep.Players) > 0 {
		if rep.PlayerLinks == nil {
			rep.PlayerLinks = map[int]int{}
		}
		rep.PlayerLinks[len(rep.Players)-1] = id
	}
}

func (rep *TurnReport) linkReferee(id int) {
	if id > 0 && len(rep.Referee) > 0 {
		if rep.RefereeLinks == nil {
			rep.RefereeLinks = map[int]int{}
		}
		rep.RefereeLinks[len(rep.Referee)-1] = id
	}
}

// replayUnit records u as the viewer saw it at contact.
func (r *Room) replayUnit(mod *Module, u *Unit, path [][2]int, ref bool) ReplayUnit {
	uv := unitView(u, mod, ref)
	if len(path) == 0 {
		path = [][2]int{{u.Col, u.Row}}
	}
	ru := ReplayUnit{Name: u.Name, Abbr: uv.Abbr, Side: u.Side, Symbol: uv.Symbol, Contact: uv.Contact,
		Enemy: mod.Side(u.Side).Enemy, Path: slices.Clone(path), Before: u.Strength}
	if !ref && !mod.Side(u.Side).Playable {
		ru.Before = 0 // players know only their own numbers
	}
	if ru.Contact {
		// Players saw a contact arrive, not where it came from or how big it was.
		ru.Name, ru.Path, ru.Before = "Contact", path[len(path)-1:], 0
	}
	return ru
}

// moveReplay records a unit's move.
func (r *Room) moveReplay(mod *Module, u *Unit, m Move, ref bool) *Replay {
	return &Replay{Referee: ref, Kind: "move", Title: u.Name + " moves", Hex: m.To(),
		Units: []ReplayUnit{r.replayUnit(mod, u, m.Path, true)}}
}

// fightReplays starts the referee's and players' replays of a fight, before
// it takes effect. finish fills in the aftermath.
func (r *Room) fightReplays(mod *Module, e *Encounter, paths map[string][][2]int) (ref, pl *Replay, finish func()) {
	ids := slices.Concat(e.Friend, e.Neutral, e.Enemy)
	title := "Fight at " + HexID(e.Col, e.Row)
	fight := func(sides [2]string, verdict string) *ReplayFight {
		return &ReplayFight{Sides: sides, Dice: e.Dice, Faces: e.Faces, Hits: e.Hits, Success: mod.Combat.Success,
			Winner: e.Winner, Verdict: verdict, Result: e.Outcome.Result}
	}
	ref = &Replay{Referee: true, Kind: "fight", Title: title, Hex: [2]int{e.Col, e.Row}}
	pl = &Replay{Kind: "fight", Title: title, Hex: [2]int{e.Col, e.Row}}
	var friendNames, enemyNames, refIDs, plIDs []string
	for _, id := range ids {
		u := r.Unit(id)
		if u == nil {
			continue
		}
		ref.Units, refIDs = append(ref.Units, r.replayUnit(mod, u, paths[id], true)), append(refIDs, id)
		if roleOf(mod, u) == "enemy" {
			enemyNames = append(enemyNames, u.Name)
		} else {
			friendNames = append(friendNames, u.Name)
		}
		if roleOf(mod, u) == "neutral" && !r.VisibleToPlayers(mod, u) {
			continue // civilians nobody knew were there
		}
		// Enemies the players hadn't seen still turn up at contact, as contacts.
		pl.Units, plIDs = append(pl.Units, r.replayUnit(mod, u, paths[id], false)), append(plIDs, id)
	}
	ref.Fight = fight([2]string{joinList(friendNames), joinList(enemyNames)}, winnerText(e))
	pl.Fight = fight([2]string{"Our side", plural(len(e.Enemy), "contact", "contacts")}, playerWinnerText(e))
	finish = func() {
		after := func(rp *Replay, ids []string) {
			for i, id := range ids {
				ru, u := &rp.Units[i], r.Unit(id)
				if u == nil {
					ru.Lost = true
					continue
				}
				if ru.Before > 0 {
					ru.After = u.Strength
				}
				if last := ru.Path[len(ru.Path)-1]; last != [2]int{u.Col, u.Row} {
					ru.End = &[2]int{u.Col, u.Row}
				}
			}
		}
		after(ref, refIDs)
		after(pl, plIDs)
	}
	return
}

// replayKey names a replay across reports: operation, turn, and number.
func replayKey(rep *TurnReport, id int) string {
	return fmt.Sprintf("%d-%d-%d", rep.Operation, rep.Turn, id)
}

// ReplayKey is replayKey for templates.
func (rv ReportView) ReplayKey(id int) string {
	return fmt.Sprintf("%d-%d-%d", rv.Operation, rv.Turn, id)
}

// ReplayData is a replay ready to draw: hexes turned into map coordinates.
type ReplayData struct {
	Kind  string           `json:"kind"`
	Title string           `json:"title"`
	Focus [2]float64       `json:"focus"`
	Units []ReplayUnitData `json:"units"`
	Fight *ReplayFight     `json:"fight,omitempty"`
}

type ReplayUnitData struct {
	ReplayUnit
	Points [][2]float64 `json:"points"`
	EndAt  *[2]float64  `json:"endAt,omitempty"`
	Hexes  []string     `json:"hexes"`
}

func replayView(rp *Replay) ReplayData {
	pt := func(h [2]int) [2]float64 {
		x, y := HexCenter(h[0], h[1])
		return [2]float64{x, y}
	}
	d := ReplayData{Kind: rp.Kind, Title: rp.Title, Focus: pt(rp.Hex), Fight: rp.Fight}
	for _, u := range rp.Units {
		ud := ReplayUnitData{ReplayUnit: u}
		for _, h := range u.Path {
			ud.Points, ud.Hexes = append(ud.Points, pt(h)), append(ud.Hexes, HexID(h[0], h[1]))
		}
		if u.End != nil {
			p := pt(*u.End)
			ud.EndAt = &p
		}
		d.Units = append(d.Units, ud)
	}
	return d
}
