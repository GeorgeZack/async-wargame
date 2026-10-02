package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"asyncwargame/internal/game"
)

func formInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(r.FormValue(name), "%")))
	return n
}

// formParty reads one side of a roll. For a unit, the number is the
// situation, added to its usual bonus; for anyone else, it's the whole bonus.
func formParty(r *http.Request, p string) game.PartyInput {
	in := game.PartyInput{Unit: r.FormValue(p + "unit"), Name: r.FormValue(p + "name"), Why: r.FormValue(p + "why")}
	n := formInt(r, p+"mod")
	if in.Unit != "" {
		in.Situation = n
	} else {
		in.Bonus = &n
	}
	return in
}

// addContest sets up a roll from the desk form: side A, and side B unless
// it's a plain check. "roll" on the submit button rolls it straight away.
func (s *Server) addContest(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		mod := s.store.Cat.Modules[room.ModuleID]
		var b *game.PartyInput
		if p := formParty(r, "b_"); p.Unit != "" || p.Name != "" {
			b = &p
		}
		c, err := room.AddContest(me, mod, r.FormValue("order"), r.FormValue("title"), formParty(r, "a_"), b, r.FormValue("secret") == "on")
		if err != nil || r.FormValue("submit") != "roll" {
			return err
		}
		return room.RollContest(me, c.ID, 0, 0, time.Now())
	})
}

// rollContest rolls on the server, or records table dice.
func (s *Server) rollContest(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.RollContest(me, r.PathValue("id"), formInt(r, "die_a"), formInt(r, "die_b"), time.Now())
	})
}

func (s *Server) deleteContest(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.DeleteContest(me, r.PathValue("id")) })
}

// setOutcome writes what happened for an order, or on its own.
func (s *Server) setOutcome(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetOutcome(me, r.FormValue("order"), r.FormValue("id"), r.FormValue("title"), r.FormValue("text"), r.FormValue("seen"))
	})
}

func (s *Server) useDraft(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.UseDraft(me, r.FormValue("order")) })
}

// editUnit is the referee changing a unit from its card or the desk. Only
// the fields present in the form change.
func (s *Server) editUnit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	str := func(name string) *string {
		if _, ok := r.Form[name]; !ok {
			return nil
		}
		v := r.FormValue(name)
		return &v
	}
	flag := func(name string) *bool {
		if _, ok := r.Form[name+"_set"]; !ok {
			return nil
		}
		v := r.FormValue(name) == "on"
		return &v
	}
	e := game.UnitEdit{Name: str("name"), At: formPos(r, "x", "y"), Brief: str("brief"), Status: str("status"), Notes: str("notes"), Kit: str("kit"),
		Known: flag("known"), Identified: flag("identified"), RadioDown: flag("radio_down"), Remove: r.FormValue("remove") == "1"}
	if v := r.FormValue("strength"); v != "" {
		n := formInt(r, "strength")
		e.Strength = &n
	}
	if _, ok := r.Form["known_to_set"]; ok {
		ids := r.Form["known_to"]
		e.KnownTo = &ids
	}
	if v := r.FormValue("quality"); v != "" {
		n := formInt(r, "quality")
		e.Quality = &n
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.EditUnit(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), e)
	})
}

// --- facts ---

func (s *Server) proposeFact(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.ProposeFact(me, s.store.Cat.Modules[room.ModuleID], r.FormValue("unit"), r.FormValue("text"), time.Now())
		return err
	})
}

func (s *Server) decideFact(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if r.FormValue("action") == "withdraw" {
			return room.WithdrawFact(me, r.PathValue("id"))
		}
		return room.DecideFact(me, r.PathValue("id"), r.FormValue("action"), r.FormValue("text"), r.FormValue("reply"),
			r.FormValue("private") == "on", time.Now())
	})
}

// setIntention records what an enemy unit means to do this turn.
func (s *Server) setIntention(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetIntention(me, s.store.Cat.Modules[room.ModuleID], r.FormValue("unit"), r.FormValue("text"))
	})
}

// askAboutOrder sends the player a question about their order.
func (s *Server) askAboutOrder(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.AskAboutOrder(me, r.FormValue("order"), r.FormValue("text"), time.Now())
	})
}

// moveUnits is the referee dragging a group of counters: unit, x, and y
// repeat once per unit, in step.
func (s *Server) moveUnits(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ids, xs, ys := r.Form["unit"], r.Form["x"], r.Form["y"]
	if len(ids) != len(xs) || len(ids) != len(ys) {
		http.Error(w, "Each unit needs an x and a y.", http.StatusBadRequest)
		return
	}
	moves := make([]game.UnitMove, len(ids))
	for i, id := range ids {
		x, errX := strconv.ParseFloat(xs[i], 64)
		y, errY := strconv.ParseFloat(ys[i], 64)
		if errX != nil || errY != nil {
			http.Error(w, "Each unit needs an x and a y.", http.StatusBadRequest)
			return
		}
		moves[i] = game.UnitMove{ID: id, At: game.Pos{X: x, Y: y}}
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if strings.HasSuffix(r.URL.Path, "/plans") || me.Role == game.RoleReferee && room.Phase == game.PhaseResolving {
			return room.PlanUnits(me, s.store.Cat.Modules[room.ModuleID], moves, time.Now())
		}
		return room.MoveUnits(me, s.store.Cat.Modules[room.ModuleID], moves)
	})
}
