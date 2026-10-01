package web

import (
	"encoding/json"
	"net/http"
	"time"

	"asyncwargame/internal/game"
)

type editorData struct {
	Title string
	*game.Editor
}

// editor shows the referee's scenario editor and event tables.
func (s *Server) editor(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var data *editorData
	var status int
	err := s.store.Read(code, func(room *game.Room) {
		me := s.member(r, room)
		switch {
		case me == nil || me.Status != game.StatusApproved:
			status = http.StatusForbidden
		case me.Role != game.RoleReferee:
			status = http.StatusForbidden
		default:
			e := game.BuildEditor(room, s.store.Cat.Scenarios[room.ScenarioID], s.store.Cat.Modules[room.ModuleID], me)
			s.seatLinks(r, e.View, me)
			e.View.SetScreen("edit")
			data = &editorData{Title: "Editor · " + room.Name, Editor: e}
		}
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if data == nil {
		http.Error(w, "Only the referee can edit the scenario.", status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, http.StatusOK, "editor", data)
}

// editOps applies a JSON batch of editor changes: {"ops": [...]}.
func (s *Server) editOps(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ops []game.EditOp `json:"ops"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "Malformed editor request.", http.StatusBadRequest)
		return
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ApplyEdits(me, s.store.Cat.Modules[room.ModuleID], body.Ops)
	})
}

func (s *Server) editText(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetTexts(me, r.FormValue("campaign"), r.FormValue("briefing"), r.FormValue("notes"))
	})
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.StartOperation(me, s.store.Cat.Modules[room.ModuleID], time.Now())
	})
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ResetToScenario(me, s.store.Cat.Scenarios[room.ScenarioID], s.store.Cat.Modules[room.ModuleID])
	})
}

func (s *Server) addTestSeat(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.AddTestSeat(me)
		return err
	})
}
