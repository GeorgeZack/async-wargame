package web

import (
	"net/http"
	"time"

	"asyncwargame/internal/game"
)

// saveOrder writes or revises an order; the submit button decides whether
// it stays a draft or is committed.
func (s *Server) saveOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.SaveOrder(me, s.store.Cat.Modules[room.ModuleID], r.FormValue("unit"), r.FormValue("text"),
			r.FormValue("mark"), r.FormValue("submit") == "commit", time.Now())
		return err
	})
}

func (s *Server) withdrawOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.WithdrawOrder(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), time.Now())
	})
}

// reviewOrder is a commander approving or returning a player's order.
func (s *Server) reviewOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ReviewOrder(me, r.PathValue("id"), r.FormValue("action"), r.FormValue("note"), time.Now())
	})
}
