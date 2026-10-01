package web

import (
	"net/http"
	"time"

	"asyncwargame/internal/game"
)

// saveOrder creates or revises an order; the submit button decides whether
// it stays a draft or is committed.
func (s *Server) saveOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.SaveOrders(me, s.store.Cat.Modules[room.ModuleID], game.OrderInput{
			Unit: r.FormValue("unit"), Task: r.FormValue("task"), Dest: r.FormValue("dest"),
			Intent: r.FormValue("intent"), Risk: r.FormValue("risk"), Fallback: r.FormValue("fallback"), Transport: r.FormValue("transport"),
			Commit: r.FormValue("submit") == "commit", WithStack: r.FormValue("stack") == "on",
			Standing: r.FormValue("standing") == "on", Then: r.FormValue("then"),
		}, time.Now())
		return err
	})
}

func (s *Server) withdrawOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.WithdrawOrder(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), time.Now())
	})
}

// confirmOrder commits a carried-over standing order as it stands.
func (s *Server) confirmOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ConfirmOrder(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), time.Now())
	})
}

func (s *Server) confirmStanding(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ConfirmAllStanding(me, s.store.Cat.Modules[room.ModuleID], time.Now())
	})
}

// reviewOrder is a commander approving or returning a player's order.
func (s *Server) reviewOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ReviewOrder(me, r.PathValue("id"), r.FormValue("action"), r.FormValue("note"), time.Now())
	})
}
