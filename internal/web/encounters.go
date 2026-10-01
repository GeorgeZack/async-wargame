package web

import (
	"net/http"
	"strconv"

	"asyncwargame/internal/game"
)

func formInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.FormValue(name))
	return n
}

// settleEncounter is the referee's ruling on one fight.
func (s *Server) settleEncounter(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		dice := [2]int{formInt(r, "dice_f"), formInt(r, "dice_e")}
		hits := [2]int{formInt(r, "hits_f"), formInt(r, "hits_e")}
		return room.SettleEncounter(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), r.FormValue("mode"), dice, hits, r.FormValue("reason"))
	})
}

func (s *Server) settleAll(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SettleAll(me, s.store.Cat.Modules[room.ModuleID])
	})
}
