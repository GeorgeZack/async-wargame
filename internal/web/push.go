package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"time"

	"asyncwargame/internal/game"
	"asyncwargame/internal/push"
)

// Web push. A member turns notifications on per device; the browser's
// subscription is kept on their membership. Game code queues notices; the
// store hands them here once the change is saved, and they go out in the
// background. A subscription the push service says is gone is dropped.

func init() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

const maxPushPerMember = 5

// serviceWorker serves sw.js from the root so it can cover every page.
func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) {
	b, err := staticFS.ReadFile("static/sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

type subscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func readSubscription(r *http.Request) (subscriptionJSON, error) {
	var sub subscriptionJSON
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&sub); err != nil {
		return sub, game.ErrBadRequest
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(sub.Endpoint) > 1024 || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		return sub, game.ErrBadRequest
	}
	return sub, nil
}

// pushSubscribe records this device's subscription for the member.
func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.opts.Push == nil {
		http.Error(w, "Notifications aren't set up on this server.", http.StatusNotImplemented)
		return
	}
	sub, err := readSubscription(r)
	if err != nil {
		http.Error(w, "That isn't a valid push subscription.", http.StatusBadRequest)
		return
	}
	found := false
	err = s.store.Touch(r.PathValue("code"), func(room *game.Room) bool {
		me := s.member(r, room)
		if me == nil || me.Status != game.StatusApproved {
			return false
		}
		found = true
		me.Push = slicesDeleteEndpoint(me.Push, sub.Endpoint)
		me.Push = append(me.Push, game.PushSub{Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth, Added: time.Now().UTC()})
		if len(me.Push) > maxPushPerMember {
			me.Push = me.Push[len(me.Push)-maxPushPerMember:]
		}
		return true
	})
	if err != nil || !found {
		http.Error(w, "You're not a member of this room.", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushUnsubscribe forgets this device.
func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	sub, err := readSubscription(r)
	if err != nil {
		http.Error(w, "That isn't a valid push subscription.", http.StatusBadRequest)
		return
	}
	s.store.Touch(r.PathValue("code"), func(room *game.Room) bool {
		me := s.member(r, room)
		if me == nil {
			return false
		}
		n := len(me.Push)
		me.Push = slicesDeleteEndpoint(me.Push, sub.Endpoint)
		return len(me.Push) != n
	})
	w.WriteHeader(http.StatusNoContent)
}

// pushTest sends the member a notification to every device they turned on.
func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) {
	var notice *game.Notice
	s.store.Read(r.PathValue("code"), func(room *game.Room) {
		if me := s.member(r, room); me != nil && len(me.Push) > 0 {
			notice = &game.Notice{Room: room.Code, Member: me.ID, Title: "Notifications are on",
				Body: "This device will hear when a turn opens, orders are due, results are in, or someone writes to you.", Screen: "people", Tag: room.Code + "/test"}
		}
	})
	if notice == nil {
		http.Error(w, "Turn notifications on for this device first.", http.StatusBadRequest)
		return
	}
	s.deliver([]game.Notice{*notice})
	w.WriteHeader(http.StatusNoContent)
}

// deliver sends notices in the background.
func (s *Server) deliver(notices []game.Notice) {
	if s.opts.Push == nil {
		return
	}
	for _, n := range notices {
		var subs []game.PushSub
		s.store.Read(n.Room, func(room *game.Room) {
			if m := room.Member(n.Member); m != nil && m.Status == game.StatusApproved {
				subs = append(subs, m.Push...)
			}
		})
		payload, _ := json.Marshal(n)
		for _, sub := range subs {
			s.pushSem <- struct{}{}
			go func(n game.Notice, sub game.PushSub) {
				defer func() { <-s.pushSem }()
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				err := s.opts.Push.Send(ctx, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, payload, 24*time.Hour)
				switch {
				case errors.Is(err, push.ErrGone):
					s.store.Touch(n.Room, func(room *game.Room) bool {
						m := room.Member(n.Member)
						if m == nil {
							return false
						}
						before := len(m.Push)
						m.Push = slicesDeleteEndpoint(m.Push, sub.Endpoint)
						return len(m.Push) != before
					})
				case err != nil:
					log.Printf("push to %s in %s: %v", n.Member, n.Room, err)
				}
			}(n, sub)
		}
	}
}

func slicesDeleteEndpoint(subs []game.PushSub, endpoint string) []game.PushSub {
	out := subs[:0:0]
	for _, x := range subs {
		if x.Endpoint != endpoint {
			out = append(out, x)
		}
	}
	return out
}
