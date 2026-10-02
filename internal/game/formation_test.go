package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFormationMovementAndMapPrivacy(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	sc := testScenario(t)
	initial := r.Unit("m1").Pos()
	for _, id := range []string{"m2", "m3", "m4"} {
		if r.Unit(id).Pos() != initial {
			t.Fatal("formation starts apart")
		}
	}
	if err := r.SplitOff(player, "m2"); err != ErrForbidden {
		t.Fatalf("player split: %v", err)
	}
	if err := r.Rejoin(player, "m2"); err != ErrForbidden {
		t.Fatalf("player rejoin: %v", err)
	}
	at := Pos{200, 180}
	if err := r.MoveUnits(ref, mod, []UnitMove{{"m1", at}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m2", "m3", "m4"} {
		if r.Unit(id).Pos() != at || r.MoveOf(id) == nil {
			t.Fatalf("member %s did not move with formation", id)
		}
	}
	v := BuildView(r, sc, mod, player)
	token := func(v *View) *Stack {
		for i := range v.Map.Stacks {
			if v.Map.Stacks[i].Top.ID == "f-lantern" {
				return &v.Map.Stacks[i]
			}
		}
		return nil
	}
	st := token(v)
	if st == nil || !st.Top.Mine || len(st.Pips) != 4 || len(st.Top.Members) != 4 {
		t.Fatalf("token: %+v", st)
	}
	r.Tick(mod, r.Deadline)
	if err := r.SplitOff(ref, "m2"); err != nil {
		t.Fatal(err)
	}
	detached := r.Unit("m2").Pos()
	r.MoveUnits(ref, mod, []UnitMove{{"m1", Pos{250, 200}}})
	if r.Unit("m2").Pos() != detached {
		t.Fatal("detached member followed formation")
	}
	fv := BuildView(r, sc, mod, ref)
	st = token(fv)
	if len(st.Top.Members) != 3 || !st.Pips[1].Detached || len(fv.Map.Tethers) != 1 || !slices.ContainsFunc(fv.Map.Stacks, func(s Stack) bool { return s.Top.ID == "m2" && s.Chit }) {
		t.Fatal("missing split chit or tether")
	}
	pv := BuildView(r, sc, mod, player)
	if len(token(pv).Top.Members) != 4 || len(pv.Map.Tethers) != 0 {
		t.Fatal("unpublished split leaked")
	}
	if err := r.Rejoin(ref, "m2"); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m2").Detached || r.Unit("m2").Pos() != r.Unit("m1").Pos() {
		t.Fatal("rejoin did not restore token")
	}
}

func TestFormationStaleContactAndNextOperation(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	u := r.Unit("m2")
	old := u.Pos()
	u.Reported = &old
	u.OutOfContact = true
	r.MoveUnits(ref, mod, []UnitMove{{"m1", Pos{300, 200}}})
	v := BuildView(r, testScenario(t), mod, player)
	if !slices.ContainsFunc(v.Map.Stacks, func(s Stack) bool { return s.Top.ID == "m2" && s.Top.Stale && s.Top.X == old.X }) {
		t.Fatal("last known position grouped into current token")
	}
	r.SplitOff(ref, "m2")
	if err := r.EndOperation(ref, testScenario(t), mod, central(t, "2026-10-01 12:00")); err != nil {
		t.Fatal(err)
	}
	if len(r.Formations) != 1 || u.Formation != "lantern" || u.Detached || u.Pos() != r.Unit("m1").Pos() {
		t.Fatal("formation did not redeploy")
	}
}

func TestOldModuleVersionStillLoads(t *testing.T) {
	r, mod, _, _ := testRoom(t)
	r.ModuleVersion = "3.0.0"
	r.Formations = nil
	for _, u := range r.Units {
		u.Formation = ""
	}
	path := filepath.Join(t.TempDir(), "rooms.json")
	b, _ := json.Marshal([]*Room{r})
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path, testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Read(r.Code, func(got *Room) {
		if got.ModuleVersion != "3.0.0" || len(got.Formations) != 0 {
			t.Fatal("existing room rewritten")
		}
		if got.ReachText(mod, got.Unit("m1")) == "" {
			t.Fatal("new module speed unavailable")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMovementReachTextAndIntention(t *testing.T) {
	r, mod, _, _ := testRoom(t)
	u := r.Unit("m1")
	reach, ok := r.ReachOf(mod, u)
	if !ok || reach.Careful <= 0 || reach.Run <= reach.Careful {
		t.Fatalf("reach: %+v", reach)
	}
	if s := r.ReachText(mod, u); !strings.Contains(s, "hexes") || !strings.Contains(s, "at a run") {
		t.Fatal(s)
	}
	if r.IntendedMove(mod, u, nil) != "" {
		t.Fatal("unmarked order has intention")
	}
	for _, tc := range []struct {
		cells float64
		want  string
	}{{reach.CarefulCells * .8, "within a turn at a careful pace"}, {(reach.CarefulCells + reach.RunCells) / 2, "within reach at a run"}, {reach.RunCells * 2, "too far for one turn even at a run"}} {
		o := &Order{Mark: &Pos{u.X + tc.cells*r.Board.Cell, u.Y}}
		if s := r.IntendedMove(mod, u, o); !strings.Contains(s, tc.want) {
			t.Errorf("%s: %s", tc.want, s)
		}
	}
	r.Board.Distance.Amount = 0
	if r.ReachText(mod, u) != "" {
		t.Fatal("unscaled board has reach")
	}
	if s := r.IntendedMove(mod, u, &Order{Mark: &Pos{u.X + r.Board.Cell, u.Y}}); strings.Contains(s, "about 0") || !strings.Contains(s, "1 hex away") {
		t.Fatal(s)
	}
}
