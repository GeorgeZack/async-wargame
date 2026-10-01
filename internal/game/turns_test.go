package game

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func central(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("America/Chicago")
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestScheduleNext(t *testing.T) {
	s := Schedule{TimeZone: "America/Chicago", Times: []string{"18:00", "00:00"}}
	cases := []struct{ from, want string }{
		{"2026-09-30 17:00", "2026-09-30 18:00"},
		{"2026-09-30 18:00", "2026-10-01 00:00"}, // strictly after
		{"2026-09-30 19:00", "2026-10-01 00:00"},
		{"2026-10-31 19:00", "2026-11-01 00:00"},
		{"2026-11-01 01:00", "2026-11-01 18:00"}, // the day daylight saving ends
	}
	for _, c := range cases {
		if got := s.Next(central(t, c.from)); !got.Equal(central(t, c.want)) {
			t.Errorf("Next(%s) = %s, want %s", c.from, got.In(s.Location()), c.want)
		}
	}
	// 6 PM CST after the change is midnight UTC, not 11 PM.
	if got := s.Next(central(t, "2026-11-01 01:00")); got.UTC().Hour() != 0 {
		t.Errorf("6 PM CST = %s UTC", got.UTC())
	}
	weekdays := Schedule{TimeZone: "America/Chicago", Times: []string{"18:00"}, Days: []int{1, 2, 3, 4, 5}}
	if got := weekdays.Next(central(t, "2026-10-02 19:00")); !got.Equal(central(t, "2026-10-05 18:00")) {
		t.Errorf("weekday schedule from Friday evening = %s, want Monday 6 PM", got.In(weekdays.Location()))
	}
}

func startedRoom(t *testing.T, now time.Time) (*Room, *Module, *Member) {
	r, mod, _, ref := testRoom(t)
	if err := r.StartOperation(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	return r, mod, ref
}

func TestTurnOpensWithMinimumPlanningTime(t *testing.T) {
	r, _, _ := startedRoom(t, central(t, "2026-09-30 17:30"))
	// The default schedule locks only at 6 PM; 6 PM today is under an hour away.
	if want := central(t, "2026-10-01 18:00"); !r.Deadline.Equal(want) {
		t.Errorf("deadline %s, want tomorrow 6 PM", r.FormatInZone(r.Deadline))
	}
}

func TestLockAndTakeover(t *testing.T) {
	start := central(t, "2026-09-30 12:00")
	r, mod, ref := startedRoom(t, start) // locks 6 PM, AI takes over 6 hours later
	if r.Due(central(t, "2026-09-30 17:59")) {
		t.Fatal("due before the deadline")
	}
	r.Tick(mod, central(t, "2026-09-30 18:00"))
	if r.Phase != PhaseResolving || r.Turn != 1 || r.Resolution == nil || r.Resolution.Turn != 1 {
		t.Fatalf("after lock: phase %s turn %d resolution %v", r.Phase, r.Turn, r.Resolution != nil)
	}
	if r.Unsettled() > 0 {
		if err := r.NextTurn(ref, mod, central(t, "2026-09-30 19:00")); err == nil {
			t.Error("published with fights unsettled")
		}
	}
	if r.Due(central(t, "2026-09-30 23:59")) {
		t.Error("takeover before its time")
	}
	res := r.Resolution
	r.Tick(mod, central(t, "2026-10-01 00:00"))
	if !res.Published || r.Turn != 2 || r.Phase != PhasePlanning || len(r.Reports) != 1 {
		t.Fatalf("after takeover: published %v, turn %d, phase %s, %d reports", res.Published, r.Turn, r.Phase, len(r.Reports))
	}
	for _, e := range res.Encounters {
		if !e.Settled || e.Source != "rule engine" {
			t.Errorf("takeover left a fight %+v", e)
		}
	}
	if want := central(t, "2026-10-01 18:00"); !r.Deadline.Equal(want) {
		t.Errorf("turn 2 locks %s", r.FormatInZone(r.Deadline))
	}
}

func TestRefereeResolvesBeforeTakeover(t *testing.T) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	r.Tick(mod, central(t, "2026-09-30 18:00"))
	publishTurn(t, r, mod, ref, central(t, "2026-09-30 20:00"))
	if r.Turn != 2 || r.Due(central(t, "2026-10-01 00:30")) {
		t.Error("the AI would still take over a turn the referee resolved")
	}
}

func TestTakeoverSettings(t *testing.T) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	never := r.Schedule
	never.Takeover = -1
	if err := r.SetSchedule(ref, never, central(t, "2026-09-30 12:00")); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, central(t, "2026-09-30 18:00"))
	if r.Due(central(t, "2026-10-10 00:00")) {
		t.Error("takeover 'never' still takes over")
	}

	r2, mod2, ref2 := startedRoom(t, central(t, "2026-09-30 12:00"))
	now := r2.Schedule
	now.Takeover = 0
	r2.SetSchedule(ref2, now, central(t, "2026-09-30 12:00"))
	r2.Tick(mod2, central(t, "2026-09-30 18:00"))
	if r2.Turn != 2 || r2.Phase != PhasePlanning {
		t.Errorf("immediate takeover: turn %d phase %s", r2.Turn, r2.Phase)
	}
}

func TestDefer(t *testing.T) {
	now := central(t, "2026-09-30 12:00")
	r, mod, ref := startedRoom(t, now)
	if err := r.Defer(ref, now.Add(time.Minute), now); err == nil {
		t.Error("deferred to a moment from now")
	}
	until := central(t, "2026-10-01 21:00")
	if err := r.Defer(ref, until, now); err != nil {
		t.Fatal(err)
	}
	if r.Due(central(t, "2026-09-30 18:00")) || !r.Due(until) || !r.Deferred {
		t.Error("deferred lock not honored")
	}
	r.Tick(mod, until)
	if r.Phase != PhaseResolving {
		t.Error("deferred turn didn't lock")
	}
	publishTurn(t, r, mod, ref, until)
	if r.Deferred {
		t.Error("the next turn inherited the deferral")
	}
}

func TestEnemyMovesRules(t *testing.T) {
	r, mod, _ := startedRoom(t, central(t, "2026-09-30 12:00"))
	// A runner just south of the river with marines straight across it. The
	// nearest bridges are out of reach this turn, so it can't cross.
	r.Units = append(r.Units, &Unit{ID: "rx", Name: "Test runners", Class: "runners", Side: "infected", Col: 10, Row: 10, Strength: 10})
	r.Units = append(r.Units, &Unit{ID: "mx", Name: "Bait", Class: "rifle_squad", Side: "marines", Col: 10, Row: 8, Strength: 10, Known: true})

	moves := r.EnemyMoves(mod, rngFor(7))
	if !reflect.DeepEqual(moves, r.EnemyMoves(mod, rngFor(7))) {
		t.Error("same seed, different moves")
	}
	people := map[[2]int]bool{}
	for _, u := range r.Units {
		if u.Side != "infected" {
			people[[2]int{u.Col, u.Row}] = true
		}
	}
	for _, m := range moves {
		u := r.Unit(m.Unit)
		cl := mod.Class(u.Class)
		from, to := m.Path[0], m.Path[len(m.Path)-1]
		if mod.TerrainAt(r.Map.TerrainCode(to[0], to[1])).Water() || people[to] {
			t.Errorf("%s ends in water or on people at %s", u.Name, HexID(to[0], to[1]))
		}
		if c := r.pathCost(mod, from[0], from[1], to[0], to[1]); c < 0 || c > cl.Movement {
			t.Errorf("%s moves %s→%s at cost %d with %d movement", u.Name, HexID(from[0], from[1]), HexID(to[0], to[1]), c, cl.Movement)
		}
		for i := 1; i < len(m.Path); i++ {
			if HexDistance(m.Path[i-1][0], m.Path[i-1][1], m.Path[i][0], m.Path[i][1]) != 1 {
				t.Errorf("%s's path jumps", u.Name)
			}
		}
		if u.ID == "rx" && to[1] < 10 {
			t.Errorf("runners crossed the river without a crossing to %s", HexID(to[0], to[1]))
		}
		if u.Class == "crawlers" || u.Class == "brood_nest" || u.Class == "screamer" {
			t.Errorf("%s should hold its ground", u.Name)
		}
	}
}

// pathCost is the cheapest movement cost between two hexes, or -1.
func (r *Room) pathCost(mod *Module, c1, r1, c2, r2 int) int {
	dist := map[[2]int]int{{c1, r1}: 0}
	frontier := [][2]int{{c1, r1}}
	for len(frontier) > 0 {
		var next [][2]int
		for _, h := range frontier {
			for dir := range 6 {
				c := r.moveCost(mod, h[0], h[1], dir)
				if c < 0 {
					continue
				}
				nc, nr, _ := r.Map.Neighbor(h[0], h[1], dir)
				n := [2]int{nc, nr}
				if d, ok := dist[n]; !ok || dist[h]+c < d {
					dist[n] = dist[h] + c
					next = append(next, n)
				}
			}
		}
		frontier = next
	}
	if d, ok := dist[[2]int{c2, r2}]; ok {
		return d
	}
	return -1
}

func TestAFightIsResolvedAndReported(t *testing.T) {
	r, mod, ref := startedRoom(t, central(t, "2026-09-30 12:00"))
	// Stragglers in the open three hexes from a squad close in and fight.
	r.Units = []*Unit{
		{ID: "m", Name: "Squad", Class: "rifle_squad", Side: "marines", Col: 22, Row: 2, Strength: 10, Known: true},
		{ID: "z", Name: "Stragglers", Class: "horde", Side: "infected", Col: 22, Row: 5, Strength: 20},
	}
	r.Tick(mod, r.Deadline)
	res := r.Resolution
	if len(res.Encounters) != 1 || res.Encounters[0].Friend[0] != "m" || res.Encounters[0].Enemy[0] != "z" {
		t.Fatalf("encounters: %+v", res.Encounters)
	}
	e := res.Encounters[0]
	// The squad could see them coming across open ground: no ambush.
	if e.Suggested[FriendSide].Dice == 0 || e.Suggested[EnemySide].Dice == 0 || e.Sit[EnemySide].Unseen {
		t.Errorf("pools %+v, unseen %v", e.Suggested, e.Sit[EnemySide].Unseen)
	}
	publishTurn(t, r, mod, ref, central(t, "2026-09-30 19:00"))
	if r.Stats.Encounters != 1 || len(r.Reports) != 1 || !slices.ContainsFunc(r.Reports[0].Players, func(l string) bool { return strings.HasPrefix(l, "Fight at") }) {
		t.Errorf("stats %+v, reports %+v", r.Stats, r.Reports)
	}
	for _, l := range r.Reports[0].Players {
		if strings.Contains(l, "Stragglers") {
			t.Errorf("player report names the infected: %q", l)
		}
	}
}

func TestFogOfWar(t *testing.T) {
	r, mod, _ := startedRoom(t, central(t, "2026-09-30 12:00"))
	now := central(t, "2026-09-30 13:00")
	r.Units = []*Unit{
		{ID: "m", Name: "Squad", Class: "rifle_squad", Side: "marines", Col: 6, Row: 4, Strength: 10, Known: true},
		{ID: "open", Name: "In the open", Class: "horde", Side: "infected", Col: 8, Row: 2, Strength: 20},    // 3 hexes, open ground
		{ID: "dense", Name: "In the blocks", Class: "horde", Side: "infected", Col: 8, Row: 5, Strength: 20}, // 2 hexes, but urban
		{ID: "far", Name: "Far away", Class: "horde", Side: "infected", Col: 20, Row: 15, Strength: 20},
	}
	r.Map.SetTerrain(8, 2, '.')
	r.Map.SetTerrain(8, 5, 'U')
	r.RefreshFog(mod, now)
	if !r.VisibleToPlayers(mod, r.Unit("open")) {
		t.Error("a horde in the open within sight range is hidden")
	}
	if r.VisibleToPlayers(mod, r.Unit("dense")) || r.VisibleToPlayers(mod, r.Unit("far")) {
		t.Error("dense cover or distance didn't hide a horde")
	}
	// It walks out of sight: players keep a last-seen marker, not the live position.
	r.Unit("open").Col, r.Unit("open").Row = 14, 1
	r.Turn = 3
	r.RefreshFog(mod, now)
	if r.VisibleToPlayers(mod, r.Unit("open")) {
		t.Error("still visible after leaving sight")
	}
	player := &Member{ID: "p", Name: "Hicks", Role: RolePlayer, Status: StatusApproved}
	r.Members = append(r.Members, player)
	v := BuildView(r, testScenario(t), mod, player)
	if d := HexDistance(6, 4, 8, 2); d != 3 {
		t.Fatalf("test setup: distance %d", d)
	}
	ghosts := v.Data.Units[HexID(8, 2)]
	if len(ghosts) != 1 || !ghosts[0].Ghost || ghosts[0].SeenTurn != 1 {
		t.Errorf("last-seen marker at 0903: %+v", ghosts)
	}
	if units := v.Data.Units[HexID(14, 1)]; len(units) != 0 {
		t.Error("players see the contact's new position")
	}
	if !strings.Contains(v.Map.Fog, "M") || v.Map.Fog == "" {
		t.Error("no fog drawn")
	}
}

func testScenario(t *testing.T) *Scenario {
	_, _, sc, _ := testRoom(t)
	return sc
}

// publishTurn settles every fight by the rules and publishes the turn.
func publishTurn(t *testing.T, r *Room, mod *Module, ref *Member, now time.Time) {
	t.Helper()
	if err := r.SettleAll(ref, mod); err != nil {
		t.Fatal(err)
	}
	if err := r.NextTurn(ref, mod, now); err != nil {
		t.Fatal(err)
	}
}
