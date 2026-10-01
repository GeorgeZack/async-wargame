package game

import (
	"strings"
	"testing"
)

func TestEmbarkRideDisembark(t *testing.T) {
	r, mod, ref, hicks := quietRoom(t,
		&Unit{ID: "m1", Name: "1st Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 6, Row: 7, Strength: 10, Known: true, AssignedTo: "h"},
		&Unit{ID: "m6", Name: "Mule", Class: "transport", Side: "marines", Col: 7, Row: 7, Strength: 3, Known: true, AssignedTo: "h"},
	)
	now := central(t, "2026-09-30 13:00")
	commit := func(in OrderInput) {
		t.Helper()
		in.Risk, in.Fallback, in.Commit = "balanced", "hold", true
		if _, err := r.SaveOrder(hicks, mod, in, now); err != nil {
			t.Fatal(err)
		}
	}
	// Turn 1: the squad walks over and climbs in.
	commit(OrderInput{Unit: "m1", Task: "embark", Transport: "m6"})
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	m1, mule := r.Unit("m1"), r.Unit("m6")
	if m1.Aboard != "m6" || m1.Col != mule.Col || m1.Row != mule.Row {
		t.Fatalf("not aboard: aboard=%q at %s", m1.Aboard, HexID(m1.Col, m1.Row))
	}
	if r.Load(mod)[[2]int{mule.Col, mule.Row}] != stackPoints(mod, mule) {
		t.Error("a passenger counts toward stacking")
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "move", Dest: HexID(9, 7), Risk: "balanced", Fallback: "hold"}, now); err == nil {
		t.Error("a passenger walked off without disembarking")
	}
	// Turn 2: the Mule drives east with the squad inside.
	var dest [2]int
	for h, st := range r.Reach(mod, mule) {
		if st.Cost > 0 && h[0] > dest[0] && h[1] == mule.Row {
			dest = h
		}
	}
	commit(OrderInput{Unit: "m6", Task: "move", Dest: HexID(dest[0], dest[1])})
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	if mule.Col != dest[0] || m1.Col != mule.Col || m1.Row != mule.Row || m1.Aboard != "m6" {
		t.Fatalf("the squad didn't ride along: mule %s, squad %s", HexID(mule.Col, mule.Row), HexID(m1.Col, m1.Row))
	}
	// Turn 3: the squad gets off next to the Mule.
	var off [2]int
	for h, st := range r.Reach(mod, m1) {
		if st.Cost == 1 {
			off = h
			break
		}
	}
	commit(OrderInput{Unit: "m1", Task: "disembark", Dest: HexID(off[0], off[1])})
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, ref, now)
	if m1.Aboard != "" || [2]int{m1.Col, m1.Row} != off {
		t.Errorf("didn't get off at %s: aboard=%q at %s", HexID(off[0], off[1]), m1.Aboard, HexID(m1.Col, m1.Row))
	}
}

func TestTransportCapacityAndDropships(t *testing.T) {
	r, mod, _, hicks := quietRoom(t,
		&Unit{ID: "m1", Name: "1st Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 4, Row: 7, Strength: 10, Known: true, AssignedTo: "h"},
		&Unit{ID: "m2", Name: "2nd Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 4, Row: 7, Strength: 10, Known: true, AssignedTo: "h"},
		&Unit{ID: "m3", Name: "Weapons Squad", Class: "hmg_squad", Side: "marines", Col: 4, Row: 7, Strength: 6, Known: true, AssignedTo: "h"},
		&Unit{ID: "m6", Name: "Mule", Class: "transport", Side: "marines", Col: 4, Row: 7, Strength: 3, Known: true},
		&Unit{ID: "d1", Name: "UNS Halyard", Class: "dropship", Side: "marines", Col: 3, Row: 7, Strength: 12, Known: true},
	)
	now := central(t, "2026-09-30 13:00")
	for _, id := range []string{"m1", "m2"} {
		if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: id, Task: "embark", Transport: "m6", Risk: "balanced", Fallback: "hold"}, now); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	_, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m3", Task: "embark", Transport: "m6", Risk: "balanced", Fallback: "hold"}, now)
	if err == nil || !strings.Contains(err.Error(), "room for 0") {
		t.Errorf("a full Mule took a third squad: %v", err)
	}
	// The dropship sits offshore; the beach next to it is a boarding hex.
	opts := r.TransportsFor(mod, r.Unit("m3"), r.Reach(mod, r.Unit("m3")))
	var ds *TransportOption
	for i := range opts {
		if opts[i].ID == "d1" {
			ds = &opts[i]
		}
	}
	if ds == nil || ds.Board != HexID(4, 7) || ds.Cost != 0 {
		t.Fatalf("can't board the dropship from the beach: %+v", opts)
	}
	if _, err := r.SaveOrder(hicks, mod, OrderInput{Unit: "m3", Task: "embark", Transport: "d1", Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	publishTurn(t, r, mod, r.Referee(), now)
	if r.Unit("m3").Aboard != "d1" || r.Unit("m3").Col != 3 {
		t.Errorf("weapons squad isn't aboard the dropship: %+v", r.Unit("m3"))
	}
}

func TestLostTransportSetsPassengersDown(t *testing.T) {
	r, mod, _, _ := quietRoom(t,
		&Unit{ID: "m1", Name: "1st Rifle Squad", Class: "rifle_squad", Side: "marines", Col: 7, Row: 7, Strength: 10, Aboard: "m6"},
		&Unit{ID: "m6", Name: "Mule", Class: "transport", Side: "marines", Col: 7, Row: 7, Strength: 3},
	)
	rep := &TurnReport{}
	r.destroy(mod, r.Unit("m6"), rep, "0808", true)
	m1 := r.Unit("m1")
	if m1 == nil || m1.Aboard != "" || m1.Strength != 7 {
		t.Errorf("passenger after the wreck: %+v", m1)
	}
}
