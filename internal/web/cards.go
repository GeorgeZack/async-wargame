package web

import (
	"net/http"
	"time"

	"asyncwargame/internal/game"
)

type cardData struct {
	Title string
	*game.View
	Cards         []*game.UnitCard
	FormationName string
	Card          *game.UnitCard
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
		v.SetScreen("map")
		d = &cardData{Title: card.Unit.Name + " · " + room.Name, View: v, Card: card, Cards: []*game.UnitCard{card}}
		if card.Unit.Formation != "" && r.URL.Query().Get("squad") == "1" {
			d.FormationName = card.FormationName
			d.Cards = nil
			for _, force := range v.Forces {
				for _, u := range force.Units {
					if u.Formation == card.Unit.Formation {
						if c, err := game.BuildUnitCard(room, s.store.Cat.Modules[room.ModuleID], me, u.ID); err == nil {
							d.Cards = append(d.Cards, c)
						}
					}
				}
			}
		}
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

func (s *Server) splitUnit(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.SplitOff(me, r.PathValue("id")) })
}
func (s *Server) rejoinUnit(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.Rejoin(me, r.PathValue("id")) })
}

func (s *Server) applyPlans(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ApplyPlans(me, s.store.Cat.Modules[room.ModuleID])
	})
}

func (s *Server) clearStagedMoves(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.ClearStagedMoves(me) })
}
