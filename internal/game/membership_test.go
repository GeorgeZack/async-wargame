package game

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLeavingHandsUnitsOn(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")
	o, err := r.SaveOrder(hicks, mod, "m1", "Dig in by the ambulance.", nil, false, now)
	if err != nil {
		t.Fatal(err)
	}

	// Nobody else can remove Hicks; he can leave himself, and his units go to
	// his commander, draft and all.
	if err := r.Remove(vasq, hicks.ID, ""); err != ErrForbidden {
		t.Errorf("Vasquez removed Hicks: %v", err)
	}
	if err := r.Remove(hicks, hicks.ID, "v"); err != nil {
		t.Fatal(err)
	}
	if hicks.Status != StatusRemoved || r.Unit("m1").AssignedTo != cmd.ID {
		t.Errorf("after leaving: %s, m1 with %q", hicks.Status, r.Unit("m1").AssignedTo)
	}
	if r.CurrentOrder("m1") != o || !r.CanOrder(mod, cmd, r.Unit("m1")) {
		t.Error("Apone can't take over the squad's draft")
	}
	if err := r.Remove(hicks, hicks.ID, ""); err == nil {
		t.Error("left twice")
	}

	// The referee removes Apone and hands everything to Vasquez; Apone's
	// players would report to the referee.
	if err := r.Remove(ref, cmd.ID, "nobody"); err == nil {
		t.Error("handed units to someone who doesn't exist")
	}
	if err := r.Remove(ref, cmd.ID, vasq.ID); err != nil {
		t.Fatal(err)
	}
	if r.Unit("m1").AssignedTo != vasq.ID {
		t.Errorf("m1 with %q, want Vasquez", r.Unit("m1").AssignedTo)
	}
	// Removing Vasquez back to the referee leaves the units unassigned.
	if err := r.Remove(ref, vasq.ID, "referee"); err != nil || r.Unit("m1").AssignedTo != "" || r.Unit("m2").AssignedTo != "" {
		t.Errorf("back to the referee: %v, m1 %q m2 %q", err, r.Unit("m1").AssignedTo, r.Unit("m2").AssignedTo)
	}
	if err := r.Remove(ref, ref.ID, ""); err == nil {
		t.Error("the referee removed themselves")
	}
}

func TestMembershipReloadKeepsSeatAndInviteAccess(t *testing.T) {
	cat := testCatalog(t)
	sc := cat.Scenarios["st-ines"]
	mod := cat.Modules[sc.Module]
	path := t.TempDir() + "/state.json"
	// Use the old, flat wire format independently of Room's struct layout.
	old, err := json.Marshal([]map[string]any{{
		"code": "LEGACY", "name": "Membership", "scenario": sc.ID, "module": mod.ID,
		"phase": "setup", "units": []any{}, "log": []any{},
		"members": []map[string]any{
			{"id": "ref", "name": "Ref", "role": "referee", "status": "approved", "tokenHash": HashToken("REF-SEAT")},
			{"id": "cmd", "name": "Commander", "role": "commander", "status": "approved"},
			{"id": "player", "name": "Hicks", "role": "player", "status": "approved", "reportsTo": "cmd", "tokenHash": HashToken("PLAYER-SEAT"), "testKey": "TEST-SEAT-SECRET"},
			{"id": "pending", "name": "Quinn", "role": "player", "status": "pending"},
		},
		"invites": []map[string]any{{"id": "invite", "token": "INVITE-SECRET", "role": "player", "reportsTo": "cmd", "maxUses": 5, "uses": 1, "joined": []string{"player"}, "expiresAt": time.Now().Add(time.Hour)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Read("LEGACY", func(r *Room) {
		if r.MemberByToken("REF-SEAT") != r.Referee() || r.MemberByToken("PLAYER-SEAT") != r.Member("player") || r.MemberByToken("") != nil || r.MemberByToken("wrong") != nil {
			t.Fatal("saved seat authentication changed")
		}
		players := r.Subordinates(r.Member("cmd"))
		if len(players) != 1 || players[0].ID != "player" || !r.nameTaken("quinn") {
			t.Fatal("saved command hierarchy or pending names lost")
		}
		inv := r.InviteByToken("INVITE-SECRET")
		if inv == nil || !inv.Open(time.Now()) || r.InviteByToken("wrong") != nil {
			t.Fatal("saved invite access changed")
		}
		view, err := json.Marshal(BuildView(r, sc, mod, r.Member("player")))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(view), "INVITE-SECRET") || strings.Contains(string(view), "TEST-SEAT-SECRET") || strings.Contains(string(view), HashToken("REF-SEAT")) || strings.Contains(string(view), HashToken("PLAYER-SEAT")) {
			t.Fatal("membership secrets reached a player view")
		}
	}); err != nil {
		t.Fatal(err)
	}
	var joined string
	if err := s.Update("LEGACY", func(r *Room) error {
		m, err := r.AcceptInvite("INVITE-SECRET", "New player", HashToken("NEW-SEAT"))
		if err == nil {
			joined = m.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire []map[string]json.RawMessage
	if err := json.Unmarshal(saved, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 || wire[0]["members"] == nil || wire[0]["invites"] == nil || wire[0]["membership"] != nil || wire[0]["Membership"] != nil {
		t.Fatal("saved membership is no longer flat")
	}
	s, err = OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Read("LEGACY", func(r *Room) {
		m := r.MemberByToken("NEW-SEAT")
		inv := r.InviteByToken("INVITE-SECRET")
		if m == nil || m.ID != joined || !m.InPlay() || m.ReportsTo != "cmd" || inv == nil || inv.Uses != 2 || len(inv.Joined) != 2 || inv.Joined[1] != joined {
			t.Fatal("invite acceptance or seat access lost after save/reload")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRoomsFromAnOlderVersionAreKept(t *testing.T) {
	cat := testCatalog(t)
	path := t.TempDir() + "/state.json"
	old := `[{"code":"OLDOLD","name":"Lantern","scenario":"dark-coast-rescue","module":"dark-coast-tactical","members":[],"units":[],"log":[]}]`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Read("OLDOLD", func(*Room) {}); err != ErrNotFound {
		t.Errorf("served a room from an older version: %v", err)
	}
	if _, err := s.Create("New", "st-ines", "Ref", HashToken("x")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"OLDOLD"`) || !strings.Contains(string(b), `"New"`) {
		t.Errorf("state file lost a room: %s", b)
	}
}
