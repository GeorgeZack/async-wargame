package web

import (
	"net/http"
	"time"

	"asyncwargame/internal/game"
)

type cardData struct {
	Title string
	*game.View
	Card *game.UnitCard
}

// unitCard shows one unit's stat card. With script, the dashboard opens it
// in a dialog; without, it's an ordinary page.
func (s *Server) unitCard(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var d *cardData
	status := http.StatusNotFound
	s.store.Read(code, func(room *game.Room) {
		me := s.member(r, room)
		if me == nil || me.Status != game.StatusApproved || room.Closed {
			status = http.StatusForbidden
			return
		}
		card, err := game.BuildUnitCard(room, s.store.Cat.Modules[room.ModuleID], me, r.PathValue("id"))
		if err != nil {
			return
		}
		v := game.BuildView(room, s.store.Cat.Scenarios[room.ScenarioID], s.store.Cat.Modules[room.ModuleID], me)
		s.seatLinks(r, v, me)
		v.SetScreen("forces")
		d = &cardData{Title: card.Unit.Name + " · " + room.Name, View: v, Card: card}
	})
	if d == nil {
		http.Error(w, "No such unit, or you can't see it.", status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, http.StatusOK, "unit", d)
}

func (s *Server) endOperation(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.EndOperation(me, s.store.Cat.Scenarios[room.ScenarioID], s.store.Cat.Modules[room.ModuleID], time.Now())
	})
}
