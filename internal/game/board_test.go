package game

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaleAndGridRefs(t *testing.T) {
	r, _, _, _ := testRoom(t)
	if got := r.Board.ScaleText(); got != "1 hex = 15 m · 1 turn = 2 minutes" {
		t.Errorf("St. Ines scale: %q", got)
	}
	bridge := center(9, 7)
	if got := r.Where(bridge, false); got != "1008, at Canal bridge" {
		t.Errorf("where: %q", got)
	}
	if got := r.DistanceText(center(3, 8), center(5, 8)); got != "about 30 m" {
		t.Errorf("two hexes: %q", got)
	}
	if (Measure{1, "minutes"}).String() != "1 minute" || (Measure{2.5, "km"}).String() != "2.5 km" {
		t.Error("measure text")
	}
}

func TestRefereeChangesTheBoard(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	hicks := &Member{ID: "h", Name: "Hicks", Role: RolePlayer, Status: StatusApproved}
	sq := BoardEdit{Background: BackgroundHex, Grid: GridSquare, Cell: 40, Distance: Measure{10, "ft"}, Turn: Measure{30, "seconds"}}
	if err := r.SetBoard(hicks, sq); err != ErrForbidden {
		t.Errorf("a player changed the board: %v", err)
	}
	if err := r.SetBoard(ref, BoardEdit{Background: BackgroundImage, Grid: GridHex}); err == nil {
		t.Error("switched to an image that was never uploaded")
	}
	if err := r.SetBoard(ref, BoardEdit{Background: BackgroundHex, Grid: GridHex, Distance: Measure{5, "furlongs"}}); err == nil {
		t.Error("accepted a made-up unit")
	}
	if err := r.SetBoard(ref, sq); err != nil {
		t.Fatal(err)
	}
	if got := r.Board.ScaleText(); got != "1 square = 10 ft · 1 turn = 30 seconds" {
		t.Errorf("scale %q", got)
	}
	if got := r.GridRef(Pos{85, 10}); got != "C1" {
		t.Errorf("square ref %q", got)
	}

	// An uploaded image keeps every counter in the same relative place.
	w, h := r.Size()
	u := r.Unit("m1")
	fx, fy := u.X/w, u.Y/h
	if err := r.SetBoardImage(ref, "map.png", 2000, 1000); err != nil {
		t.Fatal(err)
	}
	if r.Board.Background != BackgroundImage || r.Board.ImageW != 2000 {
		t.Errorf("board %+v", r.Board)
	}
	if nw, nh := r.Size(); math.Abs(u.X/nw-fx) > 1e-9 || math.Abs(u.Y/nh-fy) > 1e-9 {
		t.Errorf("Reyes moved: %v,%v of %v,%v", u.X, u.Y, nw, nh)
	}
	if r.TerrainAt(mod, u.Pos()) != nil {
		t.Error("an image has no terrain")
	}
}

func TestPrivateBriefsAndPerPlayerFog(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	if !strings.Contains(r.Unit("m1").Brief, "staff code") {
		t.Fatal("St. Ines briefs didn't load")
	}
	card := func(m *Member, id string) *UnitCard {
		c, err := BuildUnitCard(r, mod, m, id)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if card(hicks, "m1").Brief == "" || card(ref, "m1").Brief == "" {
		t.Error("the holder or the referee can't read the brief")
	}
	if card(vasq, "m1").Brief != "" || card(cmd, "m1").Brief != "" {
		t.Error("someone else read Reyes's brief")
	}
	v := BuildView(r, testScenario(t), mod, hicks)
	if len(v.MyCards) != 1 || v.MyCards[0].Brief == "" {
		t.Errorf("Hicks's page lacks his brief: %+v", v.MyCards)
	}

	// The crawler is revealed to Hicks alone: he and his commander see it.
	sees := func(m *Member) bool {
		for _, g := range BuildView(r, testScenario(t), mod, m).Forces {
			for _, u := range g.Units {
				if u.ID == "z3" || strings.HasPrefix(u.ID, "c") && u.Name == "Contact" {
					return true
				}
			}
		}
		return r.KnownBy(mod, r.Unit("z3"), m)
	}
	if err := r.EditUnit(ref, mod, "z3", UnitEdit{KnownTo: &[]string{"h", "nobody"}}); err != nil {
		t.Fatal(err)
	}
	if got := r.Unit("z3").KnownTo; len(got) != 1 || got[0] != "h" {
		t.Errorf("known to %v", got)
	}
	if !sees(hicks) || !sees(cmd) || sees(vasq) {
		t.Errorf("fog: hicks %v, apone %v, vasquez %v", sees(hicks), sees(cmd), sees(vasq))
	}
	if err := r.EditUnit(ref, mod, "z3", UnitEdit{KnownTo: &[]string{}}); err != nil || sees(hicks) {
		t.Errorf("hiding it again: %v", err)
	}
}

func TestOldSavesMoveToPoints(t *testing.T) {
	cat := testCatalog(t)
	r, _, _, _ := testRoom(t)
	b, _ := json.Marshal([]*Room{r})
	// Rewrite it the way the hex-only version saved it.
	var raw []map[string]any
	json.Unmarshal(b, &raw)
	room := raw[0]
	delete(room, "board")
	for _, key := range []string{"units", "features"} {
		for _, x := range room[key].([]any) {
			m := x.(map[string]any)
			delete(m, "x")
			delete(m, "y")
			m["col"], m["row"] = 9, 4
		}
	}
	room["units"].([]any)[0].(map[string]any)["reported"] = []any{3, 4}
	b, _ = json.Marshal(raw)
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, b, 0o644)
	s, err := OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	s.Read(r.Code, func(got *Room) {
		u := got.Units[0]
		if u.Pos() != center(9, 4) || got.Features[0].Pos() != center(9, 4) {
			t.Errorf("units at %v, places at %v", u.Pos(), got.Features[0].Pos())
		}
		if u.Reported == nil || *u.Reported != center(3, 4) {
			t.Errorf("reported %v", u.Reported)
		}
		if got.Board.ScaleText() != "1 hex = 15 m · 1 turn = 2 minutes" {
			t.Errorf("board %+v", got.Board)
		}
	})
}

func TestMoveSeveralUnitsAtOnce(t *testing.T) {
	r, mod, _, ref := testRoom(t)
	hicks := &Member{ID: "h", Name: "Hicks", Role: RolePlayer, Status: StatusApproved}
	moves := []UnitMove{{ID: "m1", At: Pos{100, 100}}, {ID: "m2", At: Pos{110, 100}}}
	if err := r.MoveUnits(hicks, mod, moves); err != ErrForbidden {
		t.Errorf("a player moved counters: %v", err)
	}
	if err := r.MoveUnits(ref, mod, moves); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").Pos() != (Pos{100, 100}) || r.Unit("m2").Pos() != (Pos{110, 100}) {
		t.Errorf("m1 %v m2 %v", r.Unit("m1").Pos(), r.Unit("m2").Pos())
	}
	if err := r.MoveUnits(ref, mod, []UnitMove{{ID: "m1", At: Pos{5, 5}}, {ID: "nobody", At: Pos{5, 5}}}); err == nil {
		t.Error("moved a unit that doesn't exist")
	}
}

func TestRefereeMovesShowAtTheDesk(t *testing.T) {
	r, mod, ref, _, hicks := orderRoom(t)
	now := central(t, "2026-09-30 13:00")
	o, _ := r.SaveOrder(hicks, mod, "m1", "Reyes heads for the bridge.", nil, true, now)
	start := r.Unit("m1").Pos()
	// While orders are open the referee sees who's in, and moves the shamblers.
	v := BuildView(r, testScenario(t), mod, ref)
	in := map[string]string{}
	for _, c := range v.CheckIns {
		in[c.Unit.ID] = c.Status
	}
	if in["m1"] != "in" || in["m2"] != "none" {
		t.Errorf("check-ins %v", in)
	}
	r.MoveUnits(ref, mod, []UnitMove{{ID: "z1", At: center(9, 7)}})
	r.Tick(mod, r.Deadline)
	// At the desk: Reyes moved twice by hand comes out as one move.
	r.EditUnit(ref, mod, "m1", UnitEdit{At: ptr(center(6, 8))})
	r.EditUnit(ref, mod, "m1", UnitEdit{At: ptr(center(8, 7))})
	d := buildDesk(r, mod, ref, AIView{})
	if len(d.Items) != 1 || d.Items[0].Order.ID != o.ID || d.Items[0].Moved == nil ||
		!strings.Contains(d.Items[0].Moved.Text, "Red Beach") || !strings.Contains(d.Items[0].Moved.Text, "0908") {
		t.Fatalf("Reyes's move: %+v", d.Items[0].Moved)
	}
	if len(d.Moves) != 1 || d.Moves[0].Unit != "Shamblers by the canal" || !strings.Contains(d.Moves[0].Text, "Canal bridge") {
		t.Errorf("other moves: %+v", d.Moves)
	}
	// Moving him back where he started forgets the move.
	r.EditUnit(ref, mod, "m1", UnitEdit{At: &start})
	if r.MoveOf("m1") != nil {
		t.Error("a move back to the start is still noted")
	}
	// Players never see the notes.
	if v := BuildView(r, testScenario(t), mod, hicks); len(v.CheckIns) != 0 {
		t.Error("a player sees the check-in list")
	}
}
