package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"asyncwargame/internal/game"
)

func (s *Server) setSchedule(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	sched := game.Schedule{TimeZone: r.FormValue("tz")}
	for _, t := range r.Form["time"] {
		if t = strings.TrimSpace(t); t != "" {
			sched.Times = append(sched.Times, t)
		}
	}
	for _, d := range r.Form["day"] {
		if n, err := strconv.Atoi(d); err == nil {
			sched.Days = append(sched.Days, n)
		}
	}
	sched.Takeover, _ = strconv.Atoi(r.FormValue("takeover"))
	if len(sched.Days) == 7 {
		sched.Days = nil
	}
	if len(r.Form["day"]) == 0 {
		s.act(w, r, func(*game.Room, *game.Member) error {
			return game.BadRequest("pick at least one day for turns to lock")
		})
		return
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.SetSchedule(me, sched, time.Now()) })
}

// deferLock pushes the lock back by a preset amount, to the next scheduled
// slot after the current deadline, or to an exact time in the room's zone.
func (s *Server) deferLock(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		now := time.Now()
		var until time.Time
		switch p := r.FormValue("preset"); p {
		case "next":
			until = room.Schedule.Next(room.Deadline)
		case "1", "3", "12", "24":
			h, _ := strconv.Atoi(p)
			until = room.Deadline.Add(time.Duration(h) * time.Hour)
		case "":
			t, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("until"), room.Schedule.Location())
			if err != nil {
				return game.BadRequest("enter a date and time to defer to")
			}
			until = t
		default:
			return game.BadRequest("unknown deferral")
		}
		return room.Defer(me, until, now)
	})
}

func (s *Server) lockNow(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.LockNow(me, s.store.Cat.Modules[room.ModuleID], time.Now())
	})
}

func (s *Server) nextTurn(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.NextTurn(me, s.store.Cat.Modules[room.ModuleID], time.Now())
	})
}

func (s *Server) closeRoom(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.Close(me, time.Now()) })
}

func (s *Server) reopenRoom(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.Reopen(me, time.Now()) })
}

// deleteRoom permanently removes a closed room; the form must repeat the room code.
func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	err := s.store.Delete(code, func(room *game.Room) error {
		me := s.member(r, room)
		if me == nil || me.Role != game.RoleReferee {
			return game.ErrForbidden
		}
		if game.NormalizeCode(r.FormValue("confirm")) != room.Code {
			return game.BadRequest("type the room code %s to confirm", game.DisplayCode(room.Code))
		}
		return nil
	})
	if err != nil {
		http.Error(w, userMessage(err), statusFor(err))
		return
	}
	if r.PathValue("key") == "" {
		http.SetCookie(w, &http.Cookie{Name: cookiePrefix + code, Path: "/", MaxAge: -1})
	}
	s.render(w, http.StatusOK, "message", map[string]any{"Title": "Room deleted", "Heading": "Room deleted",
		"Text": "The room and its record are gone for good."})
}

func (s *Server) adminMode(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetAdminMode(me, r.FormValue("on") == "1", time.Now())
	})
}

func (s *Server) setRadio(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetRadio(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), r.FormValue("radio"))
	})
}

func (s *Server) commsRules(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.SetCommsRules(me, r.FormValue("on") == "1") })
}
