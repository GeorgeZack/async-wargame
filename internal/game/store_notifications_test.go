package game

import (
	"path/filepath"
	"testing"
)

func TestAssistantUpdatesWakePlayersOnlyAtPublication(t *testing.T) {
	s, e := OpenStore(filepath.Join(t.TempDir(), "state.json"), testCatalog(t))
	if e != nil {
		t.Fatal(e)
	}
	code, e := s.Create("fanout", "st-ines", "Ref", HashToken("ref"))
	if e != nil {
		t.Fatal(e)
	}
	var playerID, refID string
	e = s.Update(code, func(r *Room) error {
		refID = r.Referee().ID
		m, err := r.RequestJoin("Player", HashToken("player"))
		if err != nil {
			return err
		}
		playerID = m.ID
		return r.Decide(r.Referee(), m.ID, "approve")
	})
	if e != nil {
		t.Fatal(e)
	}
	player, stopP := s.SubscribeMember(code, playerID)
	defer stopP()
	ref, stopR := s.SubscribeMember(code, refID)
	defer stopR()
	watcher, stopW := s.Subscribe(code)
	defer stopW()
	e = s.UpdateAssistant(code, func(r *Room) error { r.Assistant.On = true; return nil })
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-player:
		t.Fatal("private bookkeeping refreshed every player's page")
	default:
	}
	for _, ch := range []<-chan int64{ref, watcher} {
		select {
		case <-ch:
		default:
			t.Fatal("referee or cancellation watcher missed private work")
		}
	}
	e = s.UpdateAssistant(code, func(r *Room) error { r.Turn++; return nil })
	if e != nil {
		t.Fatal(e)
	}
	for _, ch := range []<-chan int64{player, ref, watcher} {
		select {
		case <-ch:
		default:
			t.Fatal("publication did not wake every subscriber")
		}
	}
	// Players need a public stalled-status update, but never private error text.
	e = s.Update(code, func(r *Room) error {
		r.Phase, r.Assistant.Automatic = PhaseResolving, true
		r.Assistant.Runs = []*Run{{Kind: RunAutoResolve, Status: RunRunning}}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	<-player
	e = s.UpdateAssistant(code, func(r *Room) error {
		r.Assistant.Runs[0].Status = RunFailed
		r.Assistant.Runs[0].Error = "private diagnostic"
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-player:
	default:
		t.Fatal("player was left waiting without the stopped status")
	}
	// Membership changes apply to an existing subscription, without reconnecting.
	e = s.Update(code, func(r *Room) error { r.Member(playerID).Role = RoleReferee; return nil })
	if e != nil {
		t.Fatal(e)
	}
	<-player
	e = s.UpdateAssistant(code, func(r *Room) error { r.Assistant.On = false; return nil })
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-player:
	default:
		t.Fatal("promoted member missed referee updates")
	}
}
