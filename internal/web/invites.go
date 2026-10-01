package web

import (
	"net/http"
	"strconv"
	"time"

	"asyncwargame/internal/game"
)

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		uses, _ := strconv.Atoi(r.FormValue("uses"))
		_, err := room.CreateInvite(me, game.Role(r.FormValue("role")), r.FormValue("reports_to"), uses)
		return err
	})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.RevokeInvite(me, r.PathValue("id")) })
}

type inviteData struct {
	Title, Room, Summary, Error, Action, Name string
	Open, Member                              bool
	MemberLink                                string
}

// invitePage shows what an invite offers and asks for a display name.
func (s *Server) invitePage(w http.ResponseWriter, r *http.Request) {
	s.renderInvite(w, r, http.StatusOK, "", "")
}

func (s *Server) renderInvite(w http.ResponseWriter, r *http.Request, status int, errMsg, name string) {
	code, token := r.PathValue("code"), r.PathValue("token")
	var d *inviteData
	s.store.Read(code, func(room *game.Room) {
		inv := room.InviteByToken(token)
		if inv == nil {
			return
		}
		d = &inviteData{Title: "Invitation · " + room.Name, Room: room.Name, Summary: room.InviteSummary(inv),
			Open: inv.Open(timeNow()), Error: errMsg, Name: name, Action: "/i/" + code + "/" + token}
		if m := s.member(r, room); m != nil && m.Status == game.StatusApproved {
			d.Member, d.MemberLink = true, "/r/"+room.Code
		}
	})
	if d == nil {
		w.Header().Set("Cache-Control", "no-store")
		s.render(w, http.StatusNotFound, "message", map[string]any{"Title": "Invitation", "Heading": "Invitation not found",
			"Text": "This invite link is not valid. Check that you copied all of it, or ask the referee for a new one."})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, status, "invite", d)
}

// acceptInvite joins the room through an invite, skipping the approval queue.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	code, token := r.PathValue("code"), r.PathValue("token")
	name, err := game.CleanName(r.FormValue("name"))
	if err != nil {
		s.renderInvite(w, r, http.StatusBadRequest, userMessage(err), r.FormValue("name"))
		return
	}
	tabSeat := r.FormValue("seat") == "tab"
	if !tabSeat {
		// Joining would replace this browser's room cookie and strand its current seat.
		var existing string
		s.store.Read(code, func(room *game.Room) {
			if m := s.member(r, room); m != nil && m.Status != game.StatusRemoved && m.Status != game.StatusDeclined {
				existing = m.Name
			}
		})
		if existing != "" {
			s.renderInvite(w, r, http.StatusConflict, "This browser is already in the room as "+existing+
				". Tick “separate seat for this tab” to join as someone else.", name)
			return
		}
	}
	memberToken, hash := game.NewToken()
	err = s.store.Update(code, func(room *game.Room) error {
		_, err := room.AcceptInvite(token, name, hash)
		return err
	})
	if err != nil {
		status := statusFor(err)
		if status == http.StatusNotFound {
			status = http.StatusBadRequest
		}
		s.renderInvite(w, r, status, userMessage(err), name)
		return
	}
	if tabSeat {
		http.Redirect(w, r, "/r/"+code+"/k/"+memberToken, http.StatusSeeOther)
		return
	}
	s.setMemberCookie(w, r, code, memberToken)
	http.Redirect(w, r, "/r/"+code, http.StatusSeeOther)
}

var timeNow = time.Now
