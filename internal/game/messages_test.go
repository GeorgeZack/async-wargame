package game

import (
	"slices"
	"testing"
)

func channelIDs(r *Room, m *Member) []string {
	var ids []string
	for _, c := range r.Channels(m) {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestChannelsAndBlindReferee(t *testing.T) {
	r, _, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	now := central(t, "2026-09-30 13:00")

	if got := channelIDs(r, hicks); !slices.Equal(got, []string{"all", "players", "cmd-c", "ref-h"}) {
		t.Errorf("Hicks's channels %v", got)
	}
	if got := channelIDs(r, vasq); !slices.Equal(got, []string{"all", "players", "ref-v"}) {
		t.Errorf("Vasquez (no commander) %v", got)
	}
	if got := channelIDs(r, cmd); !slices.Contains(got, "cmd-c") {
		t.Errorf("commander %v", got)
	}
	got := channelIDs(r, ref)
	if slices.Contains(got, "players") || slices.Contains(got, "cmd-c") || !slices.Contains(got, "ref-h") {
		t.Errorf("blind referee %v", got)
	}

	if err := r.Post(hicks, "players", "Flank them at the bridge.", now); err != nil {
		t.Fatal(err)
	}
	if err := r.Post(vasq, "cmd-c", "Let me in.", now); err != ErrForbidden {
		t.Errorf("Vasquez posted in Apone's command channel: %v", err)
	}
	if err := r.Post(vasq, "ref-h", "Snooping.", now); err != ErrForbidden {
		t.Errorf("Vasquez posted in Hicks's private channel: %v", err)
	}
	if err := r.Post(ref, "players", "Hi.", now); err != ErrForbidden {
		t.Errorf("blind referee posted to players: %v", err)
	}
	if r.Thread(ref, "players") != nil {
		t.Error("blind referee read the players' channel")
	}
	if err := r.Post(hicks, "all", "   ", now); err == nil {
		t.Error("posted an empty message")
	}

	// Admin view: read-only.
	ref.AdminMode = true
	if th := r.Thread(ref, "players"); len(th) != 1 || th[0].Author != "Hicks" {
		t.Errorf("admin view thread %+v", th)
	}
	if err := r.Post(ref, "players", "Hi.", now); err == nil {
		t.Error("referee posted in a players' channel from admin view")
	}
	ref.AdminMode = false

	// Unread counts and marks.
	r.Post(ref, "ref-h", "Your squad hears engines to the north.", now)
	unread := func(m *Member, id string) int {
		for _, c := range r.Channels(m) {
			if c.ID == id {
				return c.Unread
			}
		}
		return -1
	}
	if unread(hicks, "ref-h") != 1 || unread(vasq, "players") != 1 || unread(hicks, "players") != 0 {
		t.Errorf("unread: hicks ref %d, vasq players %d, hicks players %d", unread(hicks, "ref-h"), unread(vasq, "players"), unread(hicks, "players"))
	}
	if !r.MarkRead(hicks, "ref-h") || unread(hicks, "ref-h") != 0 || r.MarkRead(hicks, "ref-h") {
		t.Error("mark read")
	}
	if r.MarkRead(vasq, "ref-h") {
		t.Error("marked a channel Vasquez can't read")
	}
}
