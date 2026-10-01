package game

import (
	"slices"
	"testing"
)

func TestFightReplaysKeepTheFog(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	u := r.Unit("m1")
	reach := r.Reach(mod, u)
	var dest [2]int
	var route [][2]int
	for h, st := range reach {
		if p := routeTo(reach, [2]int{u.Col, u.Row}, h); st.Cost > 0 && len(p) > len(route) {
			dest, route = h, p
		}
	}
	if len(route) < 2 {
		t.Skip("no multi-hex route")
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(dest[0], dest[1]), Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Units = append(r.Units, &Unit{ID: "ambush", Name: "Ambushers", Class: "crawlers", Side: "infected", Col: route[1][0], Row: route[1][1], Strength: 8})
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	rep := r.Reports[len(r.Reports)-1]

	var refFight, plFight *Replay
	has := func(rp *Replay, name string) bool {
		return slices.ContainsFunc(rp.Units, func(x ReplayUnit) bool { return x.Name == name })
	}
	for _, id := range rep.RefereeLinks {
		if rp := rep.Replays[id-1]; rp.Kind == "fight" && has(rp, "Ambushers") {
			refFight = rp
		}
	}
	for _, id := range rep.PlayerLinks {
		rp := rep.Replays[id-1]
		if rp.Referee {
			t.Errorf("a player line links to the referee's replay %q", rp.Title)
		}
		if rp.Kind == "fight" && has(rp, u.Name) {
			plFight = rp
		}
	}
	if refFight == nil || plFight == nil {
		t.Fatalf("missing fight replays: ref %v, players %v", refFight, plFight)
	}
	if !slices.ContainsFunc(refFight.Units, func(x ReplayUnit) bool { return x.Name == "Ambushers" && x.Before == 8 }) {
		t.Errorf("referee replay lacks the ambushers: %+v", refFight.Units)
	}
	for _, x := range plFight.Units {
		if x.Name == "Ambushers" || x.Enemy && (!x.Contact || x.Before != 0 || len(x.Path) != 1) {
			t.Errorf("player replay leaks the enemy: %+v", x)
		}
	}
	if plFight.Fight == nil || len(plFight.Fight.Faces[0]) != plFight.Fight.Dice[0] {
		t.Errorf("player replay dice: %+v", plFight.Fight)
	}
	if !slices.ContainsFunc(plFight.Units, func(x ReplayUnit) bool { return x.Contact }) {
		t.Errorf("the unseen ambushers should turn up as a contact: %+v", plFight.Units)
	}
	if !slices.ContainsFunc(plFight.Units, func(x ReplayUnit) bool { return x.Name == u.Name && len(x.Path) >= 1 }) {
		t.Errorf("player replay lacks the squad: %+v", plFight.Units)
	}
	// The view hands players only their replays.
	v := BuildView(r, testScenario(t), mod, hicks)
	for key, d := range v.Replays {
		if d.Kind == "fight" && slices.ContainsFunc(d.Units, func(x ReplayUnitData) bool { return x.Name == "Ambushers" }) {
			t.Errorf("view replay %s names the ambushers", key)
		}
	}
	if len(v.Replays) == 0 {
		t.Error("no replays in the player's view")
	}
}
