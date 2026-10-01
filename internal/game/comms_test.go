package game

import (
	"math/rand/v2"
	"testing"
)

func TestCommandNet(t *testing.T) {
	r, mod, ref, _, _ := orderRoom(t)
	r.RefreshComms(mod)
	for _, u := range r.Units {
		if mod.Side(u.Side).Playable && u.OutOfContact {
			t.Errorf("%s starts out of contact at %s", u.Name, HexID(u.Col, u.Row))
		}
	}
	// A lone squad deep in the city is off the net...
	m1 := r.Unit("m1")
	m1.Col, m1.Row = 16, 6
	r.RefreshComms(mod)
	if !m1.OutOfContact {
		t.Fatal("a squad 12 hexes from the beach is still in contact")
	}
	// ...until two squads strung out between it and the beach relay for it:
	// 0B08 is 6 hexes from the beachhead, each link after that is 3.
	r.Unit("m3").Col, r.Unit("m3").Row = 10, 7
	r.Unit("m5").Col, r.Unit("m5").Row = 13, 7
	if HexDistance(10, 7, 4, 7) != 6 || HexDistance(13, 7, 10, 7) != 3 || HexDistance(16, 6, 13, 7) != 3 {
		t.Fatal("test geometry")
	}
	r.RefreshComms(mod)
	if m1.OutOfContact || r.Unit("m5").OutOfContact {
		t.Errorf("a chain of squads didn't carry the net")
	}
	// The ridge relay only works while someone holds it.
	r.Unit("m3").Col, r.Unit("m3").Row = 5, 8
	r.Unit("m5").Col, r.Unit("m5").Row = 6, 7
	m1.Col, m1.Row = 20, 3 // near the relay at 2502, far from the beach
	r.RefreshComms(mod)
	if !m1.OutOfContact {
		t.Fatal("unmanned relay put a squad on the net")
	}
	m2 := r.Unit("m2")
	m2.Col, m2.Row = 24, 2 // holds the relay
	r.RefreshComms(mod)
	if m1.OutOfContact {
		t.Error("a manned relay didn't reach the squad")
	}
	// Referee overrides and the room switch.
	if err := r.SetRadio(ref, mod, "m3", RadioDown); err != nil {
		t.Fatal(err)
	}
	r.RefreshComms(mod)
	if !r.Unit("m3").OutOfContact {
		t.Error("a unit marked lost is still in contact")
	}
	r.SetCommsRules(ref, false)
	r.RefreshComms(mod)
	if r.Unit("m1").OutOfContact || !r.Unit("m3").OutOfContact {
		t.Error("with comms rules off, everyone but the lost unit should be in contact")
	}
}

func TestOrdersOffTheNet(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t) // Hicks reports to Apone and holds m1; Vasquez holds m2
	now := central(t, "2026-09-30 13:00")
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	vasq := r.Member("v")
	sees := func(m *Member) bool {
		v := BuildView(r, testScenario(t), mod, m)
		for _, o := range v.MyOrders {
			if o.Unit.ID == "m1" && o.Order != nil {
				return true
			}
		}
		for _, o := range v.OtherOrders {
			if o.UnitID == "m1" {
				return true
			}
		}
		return false
	}
	r.RefreshComms(mod)
	if !sees(cmd) || !sees(vasq) {
		t.Fatal("in contact, the order should be shared")
	}
	r.SetRadio(ref, mod, "m1", RadioDown) // lost on patrol
	r.RefreshComms(mod)
	if !sees(hicks) {
		t.Error("the player with the unit can't see its order")
	}
	if sees(cmd) || sees(vasq) {
		t.Error("the commander or another player sees a lost unit's order")
	}
	if _, err := r.SaveOrder(cmd, mod, OrderInput{Unit: "m1", Task: "hold", Risk: "balanced", Fallback: "hold"}, now); err == nil {
		t.Error("the commander changed the order of a unit he can't reach")
	}
	ref.AdminMode = true
	if !sees(ref) {
		t.Error("the referee lost sight of the order")
	}
}

func TestBlackoutJamsRadios(t *testing.T) {
	r, mod, ref, _, _ := orderRoom(t)
	r.Threat = 3
	ev, err := r.RollEvent(ref, mod, "blackout")
	if err != nil || len(ev.Effects) == 0 {
		t.Fatalf("blackout: %v %+v", err, ev)
	}
	_ = rand.New(rand.NewPCG(1, 1))
	if err := r.ApplyEvent(ref, mod, ev.ID); err != nil {
		t.Fatal(err)
	}
	r.RefreshComms(mod)
	jammed := r.Unit(ev.Effects[0].UnitID)
	if !jammed.OutOfContact {
		t.Fatal("jammed unit still in contact")
	}
	r.Turn = jammed.JammedUntil + 1
	r.RefreshComms(mod)
	if jammed.OutOfContact {
		t.Error("the jam never wore off")
	}
}

func TestOffTheNetSightAndLastReportedPosition(t *testing.T) {
	r, mod, _, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	u := r.Unit("m1") // Hicks's squad
	sc := testScenario(t)
	r.RefreshComms(mod)
	r.RefreshFog(mod, central(t, "2026-09-30 13:00"))
	if u.OutOfContact || u.Reported == nil {
		t.Fatalf("squad should start on the net: %+v", u)
	}
	home := *u.Reported

	// The squad is lost on patrol and moves off where nobody else can see.
	u.Radio = RadioDown
	r.RefreshComms(mod)
	far := [2]int{-1, -1}
	for row := range r.Map.NumRows() {
		for col := 0; col < r.Map.Cols; col++ {
			if mod.TerrainAt(r.Map.TerrainCode(col, row)).Water() || r.observedBy(r.observers(mod))[[2]int{col, row}] {
				continue
			}
			far = [2]int{col, row}
		}
	}
	if far[0] < 0 {
		t.Skip("no hex outside everyone's sight")
	}
	u.Col, u.Row = far[0], far[1]
	r.Units = append(r.Units, &Unit{ID: "zz", Name: "Lurkers", Class: "horde", Side: "infected", Col: far[0], Row: far[1], Strength: 12})
	r.RefreshComms(mod)
	r.RefreshFog(mod, central(t, "2026-09-30 13:05"))

	if r.VisibleToPlayers(mod, r.Unit("zz")) {
		t.Error("an off-net squad's sighting reached the shared picture")
	}
	find := func(v *View, name string) *UnitView {
		for _, g := range v.Forces {
			for i := range g.Units {
				if g.Units[i].Name == name || name == "contact" && g.Units[i].Contact && !g.Units[i].Ghost && g.Units[i].Hex == HexID(far[0], far[1]) {
					return &g.Units[i]
				}
			}
		}
		return nil
	}
	// Hicks sees what his squad sees, where it really is.
	hv := BuildView(r, sc, mod, hicks)
	if s := find(hv, u.Name); s == nil || s.Hex != HexID(far[0], far[1]) || s.Stale {
		t.Errorf("Hicks's view of his squad: %+v", s)
	}
	if c := find(hv, "contact"); c == nil || !c.LocalSight {
		t.Errorf("Hicks should see the lurkers through his squad: %+v", c)
	}
	// Others see the squad where it last reported, and no lurkers.
	for _, m := range []*Member{vasq, cmd} {
		v := BuildView(r, sc, mod, m)
		if s := find(v, u.Name); s == nil || s.Hex != HexID(home[0], home[1]) || !s.Stale {
			t.Errorf("%s's view of the lost squad: %+v", m.Name, s)
		}
		if c := find(v, "contact"); c != nil {
			t.Errorf("%s sees the lurkers", m.Name)
		}
	}
	// Back on the net: the picture is shared again.
	u.Radio = RadioAuto
	u.Col, u.Row = home[0], home[1]
	r.RefreshComms(mod)
	if v := BuildView(r, sc, mod, vasq); find(v, u.Name).Stale {
		t.Error("still stale after regaining contact")
	}
}
