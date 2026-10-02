package web

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"asyncwargame/internal/game"
	"asyncwargame/internal/imageai"
)

func (s *Server) SetImageAI(c *imageai.Client) { s.imageAI = c }

func (s *Server) generateVignette(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Image description is too large.", http.StatusBadRequest)
		return
	}
	op, _ := strconv.Atoi(r.PathValue("op"))
	turn, _ := strconv.Atoi(r.PathValue("turn"))
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if me.Role != game.RoleReferee {
			return game.ErrForbidden
		}
		if s.imageAI == nil {
			return game.ErrBadRequest
		}
		return room.QueueVignette(me, op, turn, r.FormValue("prompt"), r.FormValue("caption"), s.imageAI.Provider(), s.imageAI.Model(), time.Now())
	})
}

func (s *Server) decideVignette(w http.ResponseWriter, r *http.Request) {
	op, _ := strconv.Atoi(r.PathValue("op"))
	turn, _ := strconv.Atoi(r.PathValue("turn"))
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.DecideVignette(me, op, turn, r.FormValue("id"), r.FormValue("action"))
	})
}

func (s *Server) vignetteImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	allowed := false
	s.store.Read(r.PathValue("code"), func(room *game.Room) {
		me := s.member(r, room)
		if me == nil || me.Status != game.StatusApproved {
			return
		}
		for _, d := range room.Vignettes() {
			if d.File == name && d.Status == game.RunDone && (me.Role == game.RoleReferee || d.Approved) {
				allowed = true
				break
			}
		}
	})
	if !allowed || name == "" || strings.ContainsAny(name, `/\`) {
		http.NotFound(w, r)
		return
	}
	// Visibility can be withdrawn; do not let browser caches bypass that check.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.store.MapDir(), name))
}
