package game

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Discussion threads. Every room has fixed channels:
//
//   - all: everyone, referee included.
//   - players: the players and commanders, for planning among themselves.
//   - cmd-<id>: a commander and the players who report to them.
//   - ref-<id>: one member and the referee, privately.
//
// A blind referee isn't in the players' or command channels: what players
// plan there is what blind refereeing keeps from them. In admin view they can
// read those channels too (players are told when admin view is on), but they
// can't post in them.

const (
	ChannelAll     = "all"
	ChannelPlayers = "players"
	maxMessage     = 2000
)

// Message is one post in a channel.
type Message struct {
	ID      string    `json:"id"`
	Channel string    `json:"channel"`
	Author  string    `json:"author"`
	At      time.Time `json:"at"`
	Turn    int       `json:"turn"`
	Text    string    `json:"text"`
}

// Channel is a channel as one member sees it.
type Channel struct {
	ID       string
	Name     string
	Note     string
	Unread   int
	Count    int
	CanPost  bool
	Watching bool // a referee reading a players' channel in admin view
}

// Channels lists the channels me can read, in display order.
func (r *Room) Channels(me *Member) []Channel {
	if me == nil || me.Status != StatusApproved {
		return nil
	}
	var out []Channel
	add := func(id, name, note string, post bool) {
		out = append(out, Channel{ID: id, Name: name, Note: note, CanPost: post, Watching: !post})
	}
	add(ChannelAll, "All hands", "Everyone in the room, the referee included.", true)
	ref := me.Role == RoleReferee
	switch {
	case !ref:
		add(ChannelPlayers, "Players only", "Every player and commander. The referee can't read it unless they switch to admin view.", true)
	case me.AdminMode:
		add(ChannelPlayers, "Players only", "Read-only: you're reading this in admin view.", false)
	}
	for _, m := range r.Members {
		if m.Role != RoleCommander || m.Status != StatusApproved {
			continue
		}
		name := m.Name + "'s command"
		note := fmt.Sprintf("%s and the players who report to them.", m.Name)
		switch {
		case !ref && (m.ID == me.ID || me.ReportsTo == m.ID):
			add("cmd-"+m.ID, name, note, true)
		case ref && me.AdminMode:
			add("cmd-"+m.ID, name, "Read-only: you're reading this in admin view.", false)
		}
	}
	if ref {
		for _, m := range r.Members {
			if m.Role != RoleReferee && m.Status == StatusApproved {
				add("ref-"+m.ID, "With "+m.Name, "Private: you and "+m.Name+".", true)
			}
		}
	} else {
		add("ref-"+me.ID, "With the referee", "Private: you and the referee.", true)
	}
	counts := map[string]int{}
	for _, msg := range r.Messages {
		counts[msg.Channel]++
	}
	for i := range out {
		c := &out[i]
		c.Count = counts[c.ID]
		c.Unread = max(0, c.Count-r.ReadMarks[me.ID][c.ID])
		c.Watching = c.Watching && ref
	}
	return out
}

func (r *Room) channel(me *Member, id string) (Channel, bool) {
	i := slices.IndexFunc(r.Channels(me), func(c Channel) bool { return c.ID == id })
	if i < 0 {
		return Channel{}, false
	}
	return r.Channels(me)[i], true
}

// Post adds a message to a channel me can post in.
func (r *Room) Post(me *Member, channel, text string, now time.Time) error {
	c, ok := r.channel(me, channel)
	if !ok {
		return ErrForbidden
	}
	if !c.CanPost {
		return bad("you can read this channel but not post in it")
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return bad("write something first")
	}
	if len([]rune(text)) > maxMessage {
		return bad("messages are limited to %d characters", maxMessage)
	}
	r.Messages = append(r.Messages, &Message{ID: newID(), Channel: channel, Author: me.ID, At: now.UTC(), Turn: r.Turn, Text: text})
	r.announceMessage(me, channel, text)
	r.markRead(me, channel)
	return nil
}

// MarkRead records that me has seen everything in a channel. It reports
// whether anything changed.
func (r *Room) MarkRead(me *Member, channel string) bool {
	if _, ok := r.channel(me, channel); !ok {
		return false
	}
	return r.markRead(me, channel)
}

func (r *Room) markRead(me *Member, channel string) bool {
	n := 0
	for _, m := range r.Messages {
		if m.Channel == channel {
			n++
		}
	}
	if r.ReadMarks == nil {
		r.ReadMarks = map[string]map[string]int{}
	}
	if r.ReadMarks[me.ID] == nil {
		r.ReadMarks[me.ID] = map[string]int{}
	}
	if r.ReadMarks[me.ID][channel] == n {
		return false
	}
	r.ReadMarks[me.ID][channel] = n
	return true
}

// MessageView is a message ready to show.
type MessageView struct {
	Author  string
	Role    Role
	Mine    bool
	At      time.Time
	Turn    int
	Text    string
	NewLine bool // first unread message
}

// Thread is the messages of one channel, oldest first.
func (r *Room) Thread(me *Member, channel string) []MessageView {
	if _, ok := r.channel(me, channel); !ok {
		return nil
	}
	seen := r.ReadMarks[me.ID][channel]
	var out []MessageView
	i := 0
	for _, m := range r.Messages {
		if m.Channel != channel {
			continue
		}
		mv := MessageView{At: m.At, Turn: m.Turn, Text: m.Text, Mine: m.Author == me.ID, NewLine: i == seen && i > 0}
		if a := r.memberOrNil(m.Author); a != nil {
			mv.Author, mv.Role = a.Name, a.Role
		}
		out = append(out, mv)
		i++
	}
	return out
}

// OpenChannel puts a channel on the Messages screen: the one asked for if
// the viewer can read it, otherwise the first. It returns the channel's ID.
func (v *View) OpenChannel(r *Room, me *Member, id string) string {
	if len(v.Channels) == 0 {
		return ""
	}
	v.Channel = v.Channels[0]
	for _, c := range v.Channels {
		if c.ID == id {
			v.Channel = c
		}
	}
	v.Thread = r.Thread(me, v.Channel.ID)
	return v.Channel.ID
}
