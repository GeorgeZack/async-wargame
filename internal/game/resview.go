package game

import (
	"fmt"
	"slices"
	"strings"

	"asyncwargame/internal/rules"
)

// ResolutionView is the referee's resolution desk for a locked turn.
type ResolutionView struct {
	Turn       int
	Slices     int
	Notes      []string
	Moves      []MoveView
	Encounters []EncounterView
	Unsettled  int
}

type MoveView struct {
	Name, From, To, Reason, Role string
}

type Fighter struct {
	Name, ClassName, Abbr, Side, Role, Strength string
}

type EncounterView struct {
	ID, Hex, Sector string
	Slice, Slices   int
	Friend, Enemy   []Fighter
	Situation       [2][]string // plain-language situation per side
	Suggested       [2]rules.Pool
	Dice            [2]int
	Faces           [2][]DieFace
	Hits            [2]int
	Margin          int
	WinnerText      string
	Outcome         *rules.Outcome
	Effects         string
	Settled         bool
	Source, Reason  string
}

type DieFace struct {
	Face int
	Hit  bool
}

// ReportView is a turn report in one audience's version.
type ReportView struct {
	Operation, Turn int
	Summary         string
	Lines           []ReportLine
}

// ReportLine is one line of a report, with the replay it links to (0 for none).
type ReportLine struct {
	Text   string
	Replay int
}

func (r *Room) fighter(mod *Module, id string) Fighter {
	u := r.Unit(id)
	if u == nil {
		return Fighter{Name: "(gone)"}
	}
	cl := mod.Class(u.Class)
	f := Fighter{Name: u.Name, ClassName: cl.Name, Abbr: cl.Abbr, Side: u.Side, Role: roleOf(mod, u), Strength: fmt.Sprint(u.Strength)}
	if lvl := ExperienceOf(mod, u); f.Role == "player" && lvl.Dice > 0 {
		f.Strength += " · " + lvl.Name
	}
	return f
}

func situation(s SideSituation) []string {
	var out []string
	if s.Holding {
		out = append(out, fmt.Sprintf("held its ground (cover %d)", s.Cover))
	} else {
		out = append(out, "moved into the fight")
	}
	if s.Across {
		out = append(out, "fighting across a river or ridge")
	}
	if s.Unseen {
		out = append(out, "unseen until it struck")
	}
	return out
}

func effectsText(o *rules.Outcome, winner string) string {
	if o == nil || winner == "draw" || winner == "" {
		return ""
	}
	var parts []string
	l, w := o.Loser, o.Winner
	if l.Casualties > 0 {
		parts = append(parts, fmt.Sprintf("loser loses %d%%", l.Casualties))
	}
	if l.Retreat > 0 {
		parts = append(parts, fmt.Sprintf("falls back %d", l.Retreat))
	}
	if l.Suppressed {
		parts = append(parts, "is suppressed")
	}
	if w.Casualties > 0 {
		parts = append(parts, fmt.Sprintf("winner loses %d%%", w.Casualties))
	}
	return strings.Join(parts, ", ")
}

// addResolution fills the resolution desk, map overlays, reports, and AARs.
func addResolution(v *View, r *Room, mod *Module, me *Member) {
	ref := me.Role == RoleReferee
	v.Stats = r.Stats
	for i := len(r.Reports) - 1; i >= 0 && len(v.Reports) < 8; i-- {
		rep := r.Reports[i]
		lines, links := rep.Players, rep.PlayerLinks
		if ref {
			lines, links = rep.Referee, rep.RefereeLinks
		}
		rv := ReportView{Operation: rep.Operation, Turn: rep.Turn, Summary: rep.Summary}
		for j, text := range lines {
			line := ReportLine{Text: text}
			if id := links[j]; id > 0 && id <= len(rep.Replays) && (ref || !rep.Replays[id-1].Referee) {
				line.Replay = id
				if v.Replays == nil {
					v.Replays = map[string]ReplayData{}
				}
				v.Replays[replayKey(rep, id)] = replayView(rep.Replays[id-1])
			}
			rv.Lines = append(rv.Lines, line)
		}
		v.Reports = append(v.Reports, rv)
	}
	v.AARs = slices.Clone(r.Operations)
	slices.Reverse(v.AARs)

	res := r.Resolution
	if !ref || r.Phase != PhaseResolving || res == nil || res.Turn != r.Turn {
		return
	}
	rv := &ResolutionView{Turn: res.Turn, Slices: res.Slices, Notes: res.Notes, Unsettled: r.Unsettled()}
	for _, m := range res.Moves {
		u := r.Unit(m.Unit)
		if u == nil {
			continue
		}
		f, t := m.From(), m.To()
		rv.Moves = append(rv.Moves, MoveView{Name: u.Name, From: HexID(f[0], f[1]), To: HexID(t[0], t[1]), Reason: m.Reason, Role: roleOf(mod, u)})
		pts := make([]string, len(m.Path))
		for i, h := range m.Path {
			pts[i] = point(h[0], h[1])
		}
		v.Map.ResolutionLines = append(v.Map.ResolutionLines, ResolutionLine{Points: strings.Join(pts, " "), Role: roleOf(mod, u),
			Label: fmt.Sprintf("%s: %s → %s (%s)", u.Name, HexID(f[0], f[1]), HexID(t[0], t[1]), m.Reason)})
	}
	for _, e := range res.Encounters {
		ev := EncounterView{ID: e.ID, Hex: HexID(e.Col, e.Row), Sector: r.Map.SectorAt(e.Col, e.Row), Slice: e.Slice, Slices: res.Slices,
			Situation: [2][]string{situation(e.Sit[FriendSide]), situation(e.Sit[EnemySide])}, Suggested: e.Suggested, Dice: e.Dice,
			Hits: e.Hits, Margin: e.Margin, Outcome: e.Outcome, Settled: e.Settled, Source: e.Source, Reason: e.Reason,
			WinnerText: winnerText(e), Effects: effectsText(e.Outcome, e.Winner)}
		for _, id := range slices.Concat(e.Friend, e.Neutral) {
			ev.Friend = append(ev.Friend, r.fighter(mod, id))
		}
		for _, id := range e.Enemy {
			ev.Enemy = append(ev.Enemy, r.fighter(mod, id))
		}
		for side := range 2 {
			for _, f := range e.Faces[side] {
				ev.Faces[side] = append(ev.Faces[side], DieFace{Face: f, Hit: f >= mod.Combat.Success})
			}
		}
		rv.Encounters = append(rv.Encounters, ev)
		x, y := HexCenter(e.Col, e.Row)
		v.Map.EncounterMarks = append(v.Map.EncounterMarks, EncounterMark{X: x, Y: y, Settled: e.Settled,
			Label: fmt.Sprintf("Fight at %s, slice %d", HexID(e.Col, e.Row), e.Slice)})
	}
	v.Resolution = rv
}
