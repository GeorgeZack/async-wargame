package web

import (
	"net/http"
	"net/url"
	"time"

	"asyncwargame/internal/game"
)

// postMessage adds a message to a discussion channel.
func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	channel := r.FormValue("channel")
	code := r.PathValue("code")
	err := s.store.Update(code, func(room *game.Room) error {
		me := s.member(r, room)
		if me == nil {
			return game.ErrForbidden
		}
		if room.Closed {
			return game.ErrBadRequest
		}
		return room.Post(me, channel, r.FormValue("text"), time.Now())
	})
	if err != nil {
		http.Error(w, userMessage(err), statusFor(err))
		return
	}
	if r.Header.Get("X-Requested-With") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, s.base(r, code)+"/messages?c="+url.QueryEscape(channel), http.StatusSeeOther)
}
