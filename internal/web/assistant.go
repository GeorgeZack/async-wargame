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
	if s.imageAI != nil && v.IsReferee {
		v.ImageAI.Available, v.ImageAI.Provider, v.ImageAI.Model = true, s.imageAI.Provider(), s.imageAI.Model()
	}
	if s.ai == nil || !v.IsReferee {
		return
	}
	v.AI.Available, v.AI.Provider, v.AI.Model, v.AI.Instructions = true, s.ai.Provider(), s.ai.Model(), s.ai.Instructions()
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

// assistantCancel calls off assistant work: one run (run), every run of a
// kind for a target (kind, target), or everything (all=1).
func (s *Server) assistantCancel(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.CancelRuns(me, r.FormValue("run"), r.FormValue("kind"), r.FormValue("target"), r.FormValue("all") == "1", time.Now())
		return err
	})
}

func (s *Server) decideNarrative(w http.ResponseWriter, r *http.Request) {
	op, _ := strconv.Atoi(r.PathValue("op"))
	turn, _ := strconv.Atoi(r.PathValue("turn"))
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.DecideNarrative(me, op, turn, r.FormValue("action"), r.FormValue("text"))
	})
}

func (s *Server) setAutomatic(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if s.ai == nil {
			return game.ErrBadRequest
		}
		return room.SetAutomatic(me, r.FormValue("action"), time.Now())
	})
}
