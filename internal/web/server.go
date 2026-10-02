// Package web serves the browser client: server-rendered pages that work with
// plain HTML forms, enhanced by a small script for the map and live updates.
package web

import (
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
	"asyncwargame/internal/push"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const cookiePrefix = "aw_"

// Options are deployment settings.
type Options struct {
	// TrustProxy means the server sits behind a reverse proxy (Caddy, nginx,
	// a tunnel) that terminates HTTPS and sets X-Forwarded-Proto and
	// X-Forwarded-For. Don't set it when the server faces the internet directly.
	TrustProxy bool
	// HostPassword, if set, is needed to create a room. Joining doesn't need it.
	HostPassword string
	// Push sends web push notifications; nil turns them off.
	Push *push.Sender
}

type Server struct {
	opts    Options
	guard   *failGuard
	pushSem chan struct{} // limits concurrent push sends
	ai      *ai.Client    // the assistant's model, if any
	store   *game.Store
	pages   map[string]*template.Template
	assets  map[string]string // file name -> versioned URL
	handler http.Handler
}

func New(store *game.Store, opts Options) (*Server, error) {
	s := &Server{opts: opts, guard: newFailGuard(10, time.Hour), pushSem: make(chan struct{}, 8), store: store, pages: map[string]*template.Template{}, assets: map[string]string{}}

	store.Deliver = func(n []game.Notice) { go s.deliver(n) }

	static, _ := fs.Sub(staticFS, "static")
	entries, _ := fs.ReadDir(static, ".")
	for _, e := range entries {
		b, _ := fs.ReadFile(static, e.Name())
		sum := sha256.Sum256(b)
		s.assets[e.Name()] = "/static/" + e.Name() + "?v=" + hex.EncodeToString(sum[:4])
	}

	funcs := template.FuncMap{
		"asset":       func(name string) string { return s.assets[name] },
		"f1":          func(f float64) string { return fmt.Sprintf("%.1f", f) },
		"add":         func(a, b float64) float64 { return a + b },
		"ago":         ago,
		"stamp":       func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
		"iso":         func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"list":        func(items ...string) []string { return items },
		"pair":        func(a, b any) []any { return []any{a, b} },
		"unitCtx":     func(base string, u game.UnitView) map[string]any { return map[string]any{"Base": base, "U": u} },
		"zoneOptions": func() []game.ZoneOption { return game.ZoneOptions },
		"signed":      func(n int) string { return fmt.Sprintf("%+d", n) },
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m
		},
		"weekdays": func() []string { return game.WeekdayShort },
		"split":    strings.Split,
		"seq": func(n int) []int { // 1..n
			out := make([]int, n)
			for i := range out {
				out[i] = i + 1
			}
			return out
		},
	}
	for _, page := range []string{"home", "pending", "briefing", "holding", "editor", "message", "invite", "closed", "unit",
		"screen-map", "screen-turn", "screen-events", "screen-forces", "screen-people", "screen-reports", "screen-activity", "screen-messages", "screen-room", "screen-orders", "screen-notes"} {
		t, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/map.html", "templates/shell.html", "templates/sections.html", "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		s.pages[page] = t
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("POST /rooms", s.createRoom)
	mux.HandleFunc("GET /sw.js", s.serviceWorker)
	mux.HandleFunc("POST /join", s.join)
	// Every room route also exists under /r/{code}/k/{key}/, where the key
	// identifies the seat instead of the cookie. That lets several tabs of
	// one browser act as different members, and gives each member a link
	// that opens their seat on another device.
	seat := func(pattern string, h http.HandlerFunc) {
		method, rest, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(method+" /r/{code}"+rest, h)
		mux.HandleFunc(method+" /r/{code}/k/{key}"+rest, h)
	}
	seat("GET ", s.room)
	seat("GET /live", s.events)
	for _, name := range []string{"turn", "events", "forces", "people", "messages", "reports", "activity", "room", "orders", "notes"} {
		seat("GET /"+name, s.screen(name))
	}
	seat("GET /edit", s.editor)
	seat("POST /messages", s.postMessage)
	seat("POST /withdraw", s.withdraw)
	seat("POST /leave", s.leave)
	seat("POST /push/subscribe", s.pushSubscribe)
	seat("POST /push/unsubscribe", s.pushUnsubscribe)
	seat("POST /push/test", s.pushTest)
	seat("POST /briefed", s.briefed)
	seat("POST /members/{id}/{action}", s.decide)
	seat("POST /units/{id}/assign", s.assign)
	seat("POST /game-events", s.rollEvent)
	seat("POST /game-events/{id}/{action}", s.decideEvent)
	seat("POST /threat", s.threat)
	seat("POST /test-seats", s.addTestSeat)
	seat("POST /invites", s.createInvite)
	seat("POST /turn/schedule", s.setSchedule)
	seat("POST /turn/defer", s.deferLock)
	seat("POST /turn/lock", s.lockNow)
	seat("POST /turn/next", s.nextTurn)
	seat("POST /turn/reopen", s.reopenTurn)
	seat("POST /assistant", s.setAssistant)
	seat("POST /assistant/run", s.assistantRun)
	seat("POST /assistant/draft", s.useDraft)
	seat("POST /reports/{op}/{turn}/narrative", s.decideNarrative)
	seat("GET /units/{id}", s.unitCard)
	seat("POST /operation/end", s.endOperation)
	seat("POST /orders", s.saveOrder)
	seat("POST /orders/{id}/withdraw", s.withdrawOrder)
	seat("POST /orders/{id}/review", s.reviewOrder)

	seat("POST /admin-mode", s.adminMode)
	seat("POST /units/{id}/edit", s.editUnit)
	seat("POST /desk/contests", s.addContest)
	seat("POST /desk/contests/{id}/roll", s.rollContest)
	seat("POST /desk/contests/{id}/delete", s.deleteContest)
	seat("POST /desk/outcome", s.setOutcome)
	seat("POST /facts", s.proposeFact)
	seat("POST /facts/{id}", s.decideFact)
	seat("POST /close", s.closeRoom)
	seat("POST /reopen", s.reopenRoom)
	seat("POST /delete", s.deleteRoom)
	seat("POST /invites/{id}/revoke", s.revokeInvite)
	mux.HandleFunc("GET /i/{code}/{token}", s.invitePage)
	mux.HandleFunc("POST /i/{code}/{token}", s.acceptInvite)
	seat("POST /edit/ops", s.editOps)
	seat("POST /edit/text", s.editText)
	seat("POST /start", s.start)
	seat("POST /reset", s.reset)
	fileServer := http.StripPrefix("/static/", http.FileServerFS(static))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	}))

	s.handler = http.NewCrossOriginProtection().Handler(securityHeaders(gzipText(mux)))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// --- pages ---

type roomLink struct {
	Code, CodeDisplay, Name string
	Role                    game.Role
	Status                  game.Status
}

type homeData struct {
	Title     string
	Scenarios []*game.Scenario
	Rooms     []roomLink
	Code      string
	Error     string
	// NeedPassword asks for the host password to create a room.
	NeedPassword bool
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.renderHome(w, r, http.StatusOK, homeData{Code: r.URL.Query().Get("code")})
}

func (s *Server) renderHome(w http.ResponseWriter, r *http.Request, status int, d homeData) {
	d.Title = "Async Wargame"
	d.NeedPassword = s.opts.HostPassword != ""
	d.Scenarios = s.store.Cat.ScenarioList()
	for _, c := range r.Cookies() {
		code, ok := strings.CutPrefix(c.Name, cookiePrefix)
		if !ok {
			continue
		}
		s.store.Read(code, func(room *game.Room) {
			if m := room.MemberByToken(c.Value); m != nil {
				d.Rooms = append(d.Rooms, roomLink{room.Code, game.DisplayCode(room.Code), room.Name, m.Role, m.Status})
			}
		})
	}
	s.render(w, status, "home", d)
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	if s.opts.HostPassword != "" {
		ip := s.clientIP(r)
		if s.guard.blocked(ip) {
			s.renderHome(w, r, http.StatusTooManyRequests, homeData{Error: "Too many wrong passwords. Try again in an hour."})
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte(s.opts.HostPassword)) != 1 {
			s.guard.fail(ip)
			s.renderHome(w, r, http.StatusForbidden, homeData{Error: "That isn't the host password. Ask whoever runs this server for it."})
			return
		}
	}
	name, err := game.CleanName(r.FormValue("campaign"))
	if err == nil {
		var ref string
		if ref, err = game.CleanName(r.FormValue("name")); err == nil {
			token, hash := game.NewToken()
			var code string
			if code, err = s.store.Create(name, r.FormValue("scenario"), ref, hash); err == nil {
				s.setMemberCookie(w, r, code, token)
				http.Redirect(w, r, "/r/"+code, http.StatusSeeOther)
				return
			}
		}
	}
	s.renderHome(w, r, statusFor(err), homeData{Error: userMessage(err)})
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	code := game.NormalizeCode(r.FormValue("code"))
	name, err := game.CleanName(r.FormValue("name"))
	if err != nil {
		s.renderHome(w, r, http.StatusBadRequest, homeData{Code: game.DisplayCode(code), Error: userMessage(err)})
		return
	}
	tabSeat := r.FormValue("seat") == "tab"
	// A browser that already belongs to this room just goes back to it,
	// unless it asked for a separate seat for this tab.
	if c, _ := r.Cookie(cookiePrefix + code); c != nil && !tabSeat {
		var known bool
		s.store.Read(code, func(room *game.Room) { known = room.MemberByToken(c.Value) != nil })
		if known {
			http.Redirect(w, r, "/r/"+code, http.StatusSeeOther)
			return
		}
	}
	token, hash := game.NewToken()
	err = s.store.Update(code, func(room *game.Room) error {
		_, err := room.RequestJoin(name, hash)
		return err
	})
	if err != nil {
		if errors.Is(err, game.ErrNotFound) {
			err = fmt.Errorf("%w: no room has the code %s", game.ErrBadRequest, game.DisplayCode(code))
		}
		s.renderHome(w, r, statusFor(err), homeData{Code: game.DisplayCode(code), Error: userMessage(err)})
		return
	}
	if tabSeat {
		http.Redirect(w, r, "/r/"+code+"/k/"+token, http.StatusSeeOther)
		return
	}
	s.setMemberCookie(w, r, code, token)
	http.Redirect(w, r, "/r/"+code, http.StatusSeeOther)
}

type roomData struct {
	Title string
	*game.View
	Error string
}

func (s *Server) room(w http.ResponseWriter, r *http.Request) { s.screen("map")(w, r) }

// screen serves one screen of a room: map, turn, events, forces, and so on.
// Members who aren't in play yet get the pending, briefing, or holding page.
func (s *Server) screen(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.serveScreen(w, r, name) }
}

func (s *Server) serveScreen(w http.ResponseWriter, r *http.Request, screen string) {
	code := game.NormalizeCode(r.PathValue("code"))
	if code != r.PathValue("code") && r.PathValue("key") == "" {
		http.Redirect(w, r, "/r/"+code, http.StatusMovedPermanently)
		return
	}
	base := s.base(r, code)
	var (
		page   string
		data   any
		status = http.StatusOK
	)
	opened := "" // the channel shown on the Messages screen, to mark read
	err := s.store.Read(code, func(room *game.Room) {
		me := s.member(r, room)
		switch {
		case me == nil:
			page, data = "home", nil // handled below, outside the lock
		case room.Closed:
			page, data = "closed", map[string]any{"Title": room.Name, "Room": room.Name, "Base": base,
				"Code": room.Code, "CodeDisplay": game.DisplayCode(room.Code), "IsReferee": me.Role == game.RoleReferee,
				"ClosedAt": room.FormatInZone(room.ClosedAt), "Version": room.Version}
		case me.Status == game.StatusPending:
			page, data = "pending", map[string]any{"Title": room.Name, "Room": room.Name, "Code": room.Code,
				"CodeDisplay": game.DisplayCode(room.Code), "Me": me, "Base": base, "Version": room.Version}
		case me.Status == game.StatusDeclined:
			page, data = "message", map[string]any{"Title": room.Name, "Heading": "Request declined",
				"Text": "The referee of " + room.Name + " declined your access request."}
		case me.Status == game.StatusRemoved:
			page, data = "message", map[string]any{"Title": room.Name, "Heading": "No longer a member",
				"Text": "You no longer have access to " + room.Name + ". Ask the referee if you think this is a mistake."}
		default:
			sc := s.store.Cat.Scenarios[room.ScenarioID]
			v := game.BuildView(room, sc, s.store.Cat.Modules[room.ModuleID], me)
			s.seatLinks(r, v, me)
			if s.opts.Push != nil {
				v.PushKey = s.opts.Push.PublicKey()
			}
			s.aiView(v)
			switch {
			case !me.Briefed:
				page = "briefing"
			case room.Phase == game.PhaseSetup && me.Role != game.RoleReferee:
				page = "holding" // players wait until the referee releases the scenario
			case !v.HasScreen(screen):
				page = "redirect"
			default:
				if screen == "messages" {
					opened = v.OpenChannel(room, me, r.URL.Query().Get("c"))
				}
				v.SetScreen(screen)
				page = "screen-" + screen
			}
			data = roomData{Title: room.Name, View: v}
		}
	})
	if err != nil {
		s.renderHome(w, r, http.StatusNotFound, homeData{Code: game.DisplayCode(code),
			Error: "No room has the code " + game.DisplayCode(code) + "."})
		return
	}
	if page == "redirect" {
		http.Redirect(w, r, base, http.StatusSeeOther)
		return
	}
	if page == "home" {
		if r.PathValue("key") != "" {
			status = http.StatusForbidden
		}
		s.renderHome(w, r, status, homeData{Code: game.DisplayCode(code)})
		return
	}
	if opened != "" {
		s.store.Touch(code, func(room *game.Room) bool {
			me := s.member(r, room)
			return me != nil && room.MarkRead(me, opened)
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, status, page, data)
}

// events streams the room version so open pages can refresh themselves. It
// carries no game content; the page refetches its own filtered view.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var version int64
	err := s.store.Read(code, func(room *game.Room) {
		if s.member(r, room) != nil {
			version = room.Version
		}
	})
	if err != nil || version == 0 {
		http.Error(w, "not a member", http.StatusForbidden)
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch, cancel := s.store.Subscribe(code)
	defer cancel()
	fmt.Fprintf(w, "retry: 5000\nevent: hello\ndata: %d\n\n", version)
	rc.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-ch:
			fmt.Fprintf(w, "event: update\ndata: %d\n\n", v)
		case <-tick.C:
			fmt.Fprint(w, ": keepalive\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}

// --- actions ---

func (s *Server) withdraw(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.Withdraw(me) })
}

// leave takes the member out of the operation; they land on the home page.
func (s *Server) leave(w http.ResponseWriter, r *http.Request) {
	err := s.store.Update(r.PathValue("code"), func(room *game.Room) error {
		me := s.member(r, room)
		if me == nil {
			return game.ErrForbidden
		}
		return room.Remove(me, me.ID, "")
	})
	if err != nil {
		http.Error(w, userMessage(err), statusFor(err))
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) briefed(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		if me.Status != game.StatusApproved {
			return game.ErrForbidden
		}
		me.Briefed = true
		return nil
	})
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		switch r.PathValue("action") {
		case "reports-to":
			return room.SetReportsTo(me, r.PathValue("id"), r.FormValue("commander"))
		case "remove":
			return room.Remove(me, r.PathValue("id"), r.FormValue("heir"))
		}
		return room.Decide(me, r.PathValue("id"), r.PathValue("action"))
	})
}

func (s *Server) assign(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.Assign(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"), r.FormValue("member"))
	})
}

// rollEvent draws a random event, or the one named in the form, as a proposal.
func (s *Server) rollEvent(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		_, err := room.RollEvent(me, s.store.Cat.Modules[room.ModuleID], r.FormValue("event"))
		return err
	})
}

func (s *Server) decideEvent(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		switch r.PathValue("action") {
		case "apply":
			return room.ApplyEvent(me, s.store.Cat.Modules[room.ModuleID], r.PathValue("id"))
		case "discard":
			return room.DiscardEvent(me, r.PathValue("id"))
		}
		return game.ErrNotFound
	})
}

func (s *Server) threat(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		delta, err := strconv.Atoi(r.FormValue("delta"))
		if err != nil {
			return game.ErrBadRequest
		}
		return room.SetThreat(me, delta)
	})
}

// act runs a member action and redirects back to the room. Script-driven
// requests (X-Requested-With) get a status code instead of a redirect.
func (s *Server) act(w http.ResponseWriter, r *http.Request, fn func(*game.Room, *game.Member) error) {
	code := r.PathValue("code")
	reopening := strings.HasSuffix(r.URL.Path, "/reopen")
	err := s.store.Update(code, func(room *game.Room) error {
		me := s.member(r, room)
		if me == nil {
			return game.ErrForbidden
		}
		if room.Closed && !reopening {
			return fmt.Errorf("%w: this room is closed", game.ErrBadRequest)
		}
		return fn(room, me)
	})
	if err != nil {
		http.Error(w, userMessage(err), statusFor(err))
		return
	}
	if r.Header.Get("X-Requested-With") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	back := s.base(r, code)
	if r.FormValue("back") == "edit" {
		back += "/edit"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// --- helpers ---

// member identifies the requester: by the seat key in the URL when present,
// otherwise by the room cookie.
func (s *Server) member(r *http.Request, room *game.Room) *game.Member {
	if key := r.PathValue("key"); key != "" {
		return room.MemberByToken(key)
	}
	c, err := r.Cookie(cookiePrefix + room.Code)
	if err != nil {
		return nil
	}
	return room.MemberByToken(c.Value)
}

// base is the URL prefix for the requester's seat in a room.
func (s *Server) base(r *http.Request, code string) string {
	if key := r.PathValue("key"); key != "" {
		return "/r/" + code + "/k/" + key
	}
	return "/r/" + code
}

// seatLinks fills in the URLs a view needs for the requester's seat.
func (s *Server) seatLinks(r *http.Request, v *game.View, me *game.Member) {
	v.Base = s.base(r, v.Code)
	token := r.PathValue("key")
	if token == "" {
		if c, err := r.Cookie(cookiePrefix + v.Code); err == nil {
			token = c.Value
		}
	}
	scheme := "http"
	if s.https(r) {
		scheme = "https"
	}
	origin := scheme + "://" + r.Host
	v.AccessLink = origin + "/r/" + v.Code + "/k/" + token
	for i := range v.Invites {
		v.Invites[i].Link = origin + v.Invites[i].Link
	}
}

func (s *Server) setMemberCookie(w http.ResponseWriter, r *http.Request, code, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookiePrefix + code, Value: token, Path: "/",
		MaxAge: 365 * 24 * 3600, HttpOnly: true, Secure: s.https(r), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) render(w http.ResponseWriter, status int, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf strings.Builder
	if err := s.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	io.WriteString(w, buf.String())
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, game.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, game.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, game.ErrBadRequest), errors.Is(err, game.ErrRoomFull):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func userMessage(err error) string {
	if statusFor(err) == http.StatusInternalServerError {
		log.Printf("error: %v", err)
		return "Something went wrong on the server."
	}
	msg := strings.ReplaceAll(err.Error(), game.ErrBadRequest.Error()+": ", "")
	return strings.ToUpper(msg[:1]) + msg[1:]
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	zw          *gzip.Writer
	wroteHeader bool
	noBody      bool
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	h := g.Header()
	h.Add("Vary", "Accept-Encoding")
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified || h.Get("Content-Encoding") != "" {
		g.noBody = true
	} else {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.zw = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	g.WriteHeader(http.StatusOK)
	if g.zw == nil {
		return g.ResponseWriter.Write(b)
	}
	return g.zw.Write(b)
}

// gzipText compresses page and asset responses. The live stream is left alone
// so each message is delivered immediately.
func gzipText(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || strings.HasSuffix(r.URL.Path, "/live") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		next.ServeHTTP(gw, r)
		if gw.zw != nil {
			gw.zw.Close()
		}
	})
}

// reopenTurn is the referee rolling back the last publish to correct it.
func (s *Server) reopenTurn(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.ReopenTurn(me, r.FormValue("reason"), time.Now())
	})
}
