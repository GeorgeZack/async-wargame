package web

import (
	"net/http"
	"strconv"
	"time"

	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

// SetAI tells the server which model the assistant uses, so referees see
// where their game data goes. The work itself is done by assist.Worker.
func (s *Server) SetAI(c *ai.Client) { s.ai = c }

func (s *Server) aiView(v *game.View) {
	if s.ai == nil || !v.IsReferee {
		return
	}
	v.AI.Available, v.AI.Provider, v.AI.Model = true, s.ai.Provider(), s.ai.Model()
}

func (s *Server) setAssistant(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if s.ai == nil {
			return game.ErrBadRequest
		}
		return room.SetAssistant(me, r.FormValue("on") == "on", r.FormValue("suggest") == "on", r.FormValue("narrative") == "on")
	})
}

func (s *Server) assistantRun(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if s.ai == nil {
			return game.ErrBadRequest
		}
		return room.RunNow(me, r.FormValue("kind"), r.FormValue("target"), time.Now())
	})
}

func (s *Server) decideNarrative(w http.ResponseWriter, r *http.Request) {
	op, _ := strconv.Atoi(r.PathValue("op"))
	turn, _ := strconv.Atoi(r.PathValue("turn"))
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.DecideNarrative(me, op, turn, r.FormValue("action"), r.FormValue("text"))
	})
}
