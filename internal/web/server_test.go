package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"asyncwargame/content"
	"asyncwargame/internal/game"
)

func newTestServer(t *testing.T) (*httptest.Server, *game.Store) {
	t.Helper()
	cat, err := game.LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	store, err := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(store, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, store
}

// browser is a client with its own cookie jar, like a separate person's device.
type browser struct {
	t  *testing.T
	ts *httptest.Server
	c  *http.Client
}

func newBrowser(t *testing.T, ts *httptest.Server) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t, ts, &http.Client{Jar: jar}}
}

func (b *browser) post(path string, form url.Values) (*http.Response, string) {
	b.t.Helper()
	req, _ := http.NewRequest("POST", b.ts.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.ts.URL)
	res, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(body)
}

// room fetches every screen of a room and returns them as one body, so a
// check passes (or a leak fails) wherever on the room the text appears. The
// response is the main screen's.
func (b *browser) room(base string) (*http.Response, string) {
	b.t.Helper()
	res, body := b.get(base)
	for _, screen := range []string{"turn", "events", "forces", "people", "reports", "activity", "room", "orders"} {
		_, more := b.get(base + "/" + screen)
		body += more
	}
	return res, body
}

func (b *browser) get(path string) (*http.Response, string) {
	b.t.Helper()
	res, err := b.c.Get(b.ts.URL + path)
	if err != nil {
		b.t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(body)
}

func mustContain(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("page is missing %q", w)
		}
	}
}

func mustNotContain(t *testing.T, body string, bad ...string) {
	t.Helper()
	for _, w := range bad {
		if strings.Contains(body, w) {
			t.Errorf("page leaks %q", w)
		}
	}
}

func TestJoinApproveAssignFlow(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	player := newBrowser(t, ts)

	res, body := ref.post("/rooms", url.Values{"campaign": {"Lantern"}, "scenario": {"dark-coast-rescue"}, "name": {"Ref"}})
	if res.StatusCode != 200 {
		t.Fatalf("create room: %d", res.StatusCode)
	}
	code := strings.TrimPrefix(res.Request.URL.Path, "/r/")
	mustContain(t, body, "Horde A", "Suspected hive")
	_, body = ref.room("/r/" + code)
	mustContain(t, body, "Access requests", "Referee notes")

	// Lowercase code with a dash is accepted.
	lower := strings.ToLower(code[:3] + "-" + code[3:])
	_, body = player.post("/join", url.Values{"code": {lower}, "name": {"Vasquez"}})
	mustContain(t, body, "Waiting for the referee")
	mustNotContain(t, body, "Horde", "St. Ines Hospital")

	// Duplicate display names are rejected.
	other := newBrowser(t, ts)
	res, body = other.post("/join", url.Values{"code": {code}, "name": {"vasquez"}})
	if res.StatusCode != 400 || !strings.Contains(body, "already uses that name") {
		t.Errorf("duplicate name: %d", res.StatusCode)
	}

	// Pending players cannot act as referee.
	var playerID, unitID = "", "m1"
	store.Read(code, func(r *game.Room) {
		for _, m := range r.Members {
			if m.Name == "Vasquez" {
				playerID = m.ID
			}
		}
	})
	res, _ = player.post("/r/"+code+"/members/"+playerID+"/approve", nil)
	if res.StatusCode != 403 {
		t.Errorf("player self-approve: got %d, want 403", res.StatusCode)
	}

	_, body = ref.room("/r/" + code)
	mustContain(t, body, "Vasquez", "1 access request waiting")

	ref.post("/r/"+code+"/members/"+playerID+"/approve", nil)
	_, body = player.room("/r/" + code)
	mustContain(t, body, "Briefing", "Join the operation")

	// During setup players wait on a holding page with no map.
	_, body = player.post("/r/"+code+"/briefed", nil)
	mustContain(t, body, "Standing by")
	mustNotContain(t, body, "map-data", "Contact at")
	res, _ = player.get("/r/" + code + "/edit")
	if res.StatusCode != 403 {
		t.Errorf("player opened the editor: %d", res.StatusCode)
	}

	ref.post("/r/"+code+"/units/"+unitID+"/assign", url.Values{"member": {playerID}})
	_, body = player.room("/r/" + code)
	mustContain(t, body, "Standing by", "1st Rifle Squad")
	if res, _ := player.post("/r/"+code+"/start", nil); res.StatusCode != 403 {
		t.Errorf("player started the operation: %d", res.StatusCode)
	}
	ref.post("/r/"+code+"/start", nil)
	_, body = player.room("/r/" + code)
	mustContain(t, body, "Give your units orders: 0 of 1 committed", "1st Rifle Squad", "Contact at 1111", "Hospital survivors")
	// Hidden state stays on the server.
	mustNotContain(t, body, "Horde", "Stragglers", "School shelter", "Suspected hive", "Referee notes", "hive", "requested access")

	// Infected units can't be assigned.
	res, _ = ref.post("/r/"+code+"/units/z1/assign", url.Values{"member": {playerID}})
	if res.StatusCode != 400 {
		t.Errorf("assign enemy unit: got %d, want 400", res.StatusCode)
	}

	// Removing a player unassigns their units and closes access.
	ref.post("/r/"+code+"/members/"+playerID+"/remove", nil)
	_, body = player.room("/r/" + code)
	mustContain(t, body, "No longer a member")
	store.Read(code, func(r *game.Room) {
		if r.Unit(unitID).AssignedTo != "" {
			t.Error("unit still assigned after removal")
		}
	})
}

func TestUnknownRoomAndCrossOrigin(t *testing.T) {
	ts, _ := newTestServer(t)
	b := newBrowser(t, ts)
	res, body := b.post("/join", url.Values{"code": {"ZZZ-ZZZ"}, "name": {"Hicks"}})
	if res.StatusCode != 400 || !regexp.MustCompile(`No room has the code ZZZ-ZZZ`).MatchString(body) {
		t.Errorf("unknown room: %d", res.StatusCode)
	}

	req, _ := http.NewRequest("POST", ts.URL+"/join", strings.NewReader("code=AAAAAA&name=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Errorf("cross-site POST: got %d, want 403", res.StatusCode)
	}
}

func TestStatePersists(t *testing.T) {
	cat, _ := game.LoadCatalog(content.FS)
	path := filepath.Join(t.TempDir(), "state.json")
	s1, _ := game.OpenStore(path, cat)
	code, err := s1.Create("Lantern", "dark-coast-rescue", "Ref", game.HashToken("tok"))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := game.OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Read(code, func(r *game.Room) {
		if r.MemberByToken("tok") == nil {
			t.Error("referee token lost after reload")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// joinRoom creates a room as ref and returns its code.
func createRoom(t *testing.T, ref *browser) string {
	t.Helper()
	res, _ := ref.post("/rooms", url.Values{"campaign": {"Lantern"}, "scenario": {"dark-coast-rescue"}, "name": {"Ref"}})
	return strings.TrimPrefix(res.Request.URL.Path, "/r/")
}

func TestTabSeatsActAsSeparateMembers(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)

	// One browser (one cookie jar), two tabs, two seats.
	b := newBrowser(t, ts)
	res1, _ := b.post("/join", url.Values{"code": {code}, "name": {"Hicks"}, "seat": {"tab"}})
	res2, _ := b.post("/join", url.Values{"code": {code}, "name": {"Hudson"}, "seat": {"tab"}})
	p1, p2 := res1.Request.URL.Path, res2.Request.URL.Path
	if p1 == p2 || !strings.Contains(p1, "/k/") || !strings.Contains(p2, "/k/") {
		t.Fatalf("tab seats share a URL: %s %s", p1, p2)
	}
	_, body := b.get(p1)
	mustContain(t, body, "Hicks", "Waiting for the referee")
	_, body = b.get(p2)
	mustContain(t, body, "Hudson")
	mustNotContain(t, body, "Hicks</strong>")
	// The cookie-identified seat is still nobody: tab seats don't set cookies.
	_, body = b.room("/r/" + code)
	mustContain(t, body, "Join a room")

	// Withdrawing in one tab leaves the other.
	b.post(p2+"/withdraw", nil)
	store.Read(code, func(r *game.Room) {
		for _, m := range r.Members {
			if m.Name == "Hicks" && m.Status != game.StatusPending || m.Name == "Hudson" && m.Status != game.StatusRemoved {
				t.Errorf("%s is %s", m.Name, m.Status)
			}
		}
	})

	// A bad key is not a seat.
	res, _ := b.get("/r/" + code + "/k/" + strings.Repeat("0", 64))
	if res.StatusCode != 403 {
		t.Errorf("bad key: %d", res.StatusCode)
	}
}

func TestTestSeatsAndAccessLink(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	ref.post("/r/"+code+"/test-seats", nil)
	ref.post("/r/"+code+"/test-seats", nil)
	_, body := ref.room("/r/" + code)
	links := regexp.MustCompile(`/r/`+code+`/k/[0-9a-f]{64}`).FindAllString(body, -1)
	seen := map[string]bool{}
	for _, l := range links {
		seen[l] = true
	}
	if len(seen) != 3 { // two test seats plus the referee's own access link
		t.Fatalf("want 3 distinct seat links, got %d", len(seen))
	}
	// A test seat opens in a fresh browser without cookies and is a player.
	var testerLink string
	store.Read(code, func(r *game.Room) {
		for _, m := range r.Members {
			if m.Name == "Tester 1" {
				testerLink = "/r/" + code + "/k/" + m.TestKey
			}
		}
	})
	fresh := newBrowser(t, ts)
	_, body = fresh.get(testerLink)
	mustContain(t, body, "Tester 1", "Join the operation")
	// Players never see test seat keys.
	fresh.post(testerLink+"/briefed", nil)
	ref.post("/r/"+code+"/start", nil)
	_, body = fresh.room(testerLink)
	mustContain(t, body, "Tester 2")
	mustNotContain(t, body, "Test seats")
	keys := map[string]bool{}
	for _, k := range regexp.MustCompile(`/k/[0-9a-f]+`).FindAllString(body, -1) {
		keys[k] = true
	}
	if len(keys) != 1 { // only the tester's own seat
		t.Errorf("player page includes %d distinct seat keys", len(keys))
	}
}

func postJSON(b *browser, path, body string) (*http.Response, string) {
	b.t.Helper()
	req, _ := http.NewRequest("POST", b.ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", b.ts.URL)
	req.Header.Set("X-Requested-With", "fetch")
	res, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(out)
}

func TestEditorOps(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	path := "/r/" + code + "/edit/ops"

	_, body := ref.get("/r/" + code + "/edit")
	mustContain(t, body, "Random event tables", "Horde wave", "Brood Mother", "Rivers &amp; ridges")

	ok := func(ops string) {
		t.Helper()
		if res, msg := postJSON(ref, path, `{"ops":`+ops+`}`); res.StatusCode != 204 {
			t.Fatalf("%s: %d %s", ops, res.StatusCode, msg)
		}
	}
	fail := func(ops, want string) {
		t.Helper()
		res, msg := postJSON(ref, path, `{"ops":`+ops+`}`)
		if res.StatusCode != 400 || !strings.Contains(msg, want) {
			t.Fatalf("%s: got %d %q, want 400 containing %q", ops, res.StatusCode, msg, want)
		}
	}

	ok(`[{"op":"terrain","hex":"1010","terrain":"forest"},
	     {"op":"edge","hex":"1010","dir":1,"type":"stream"},
	     {"op":"crossing","hex":"1010","dir":1,"crossing":"ford"},
	     {"op":"road","hex":"1010","dir":0,"on":true},
	     {"op":"unit-add","class":"brute","hex":"1212","strength":1},
	     {"op":"feature-add","kind":"shelter","hex":"1313","name":"Chapel","hidden":true}]`)
	store.Read(code, func(r *game.Room) {
		if c := r.Map.TerrainCode(9, 9); c != 'f' {
			t.Errorf("terrain = %c", c)
		}
		// Stored canonically on the neighbor (north-east side of 1010 = south-west side of its neighbor).
		e := r.Map.EdgeAt(game.HexSide{Col: 9, Row: 9, Dir: 1})
		if e == nil || e.Type != "stream" || e.Crossing != "ford" {
			t.Errorf("edge = %+v", e)
		}
		if !r.Map.HasRoad(game.HexSide{Col: 10, Row: 9, Dir: 3}) {
			t.Error("road not visible from the other hex")
		}
	})
	fail(`[{"op":"crossing","hex":"1010","dir":1,"crossing":"pass"}]`, "can't have a pass")
	fail(`[{"op":"unit-add","class":"rifle_squad","hex":"0101"}]`, "can't be on water")
	fail(`[{"op":"terrain","hex":"0308","terrain":"open"}]`, "can't be on land") // a dropship's hex
	fail(`[{"op":"edge","hex":"0101","dir":2,"type":"river"}]`, "boundary")
	fail(`[{"op":"road","hex":"1010","dir":9,"on":true}]`, "boundary")
	fail(`[{"op":"road","hex":"0408","dir":3,"on":true}]`, "land on both sides")
	// A failing batch changes nothing.
	fail(`[{"op":"terrain","hex":"1111","terrain":"marsh"},{"op":"unit-delete","unit":"nope"}]`, "")
	store.Read(code, func(r *game.Room) {
		if r.Map.TerrainCode(10, 10) == 'm' {
			t.Error("partial batch was applied")
		}
	})

	// Players' maps get the stream and road once released; the hidden chapel stays hidden.
	player := newBrowser(t, ts)
	player.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	var pid string
	store.Read(code, func(r *game.Room) { pid = r.Members[1].ID })
	ref.post("/r/"+code+"/members/"+pid+"/approve", nil)
	player.post("/r/"+code+"/briefed", nil)
	ref.post("/r/"+code+"/start", nil)
	_, body = player.room("/r/" + code)
	mustContain(t, body, `class="e-stream"`, `class="x-ford"`)
	mustNotContain(t, body, "Chapel", "Brute")
	res, _ := postJSON(player, path, `{"ops":[{"op":"terrain","hex":"1010","terrain":"open"}]}`)
	if res.StatusCode != 403 {
		t.Errorf("player edit: %d", res.StatusCode)
	}
}

func memberByName(store *game.Store, code, name string) (m game.Member) {
	store.Read(code, func(r *game.Room) {
		for _, x := range r.Members {
			if x.Name == name {
				m = *x
			}
		}
	})
	return
}

// newestInvite returns the path of the most recently created invite.
func newestInvite(store *game.Store, code string) (path string) {
	store.Read(code, func(r *game.Room) {
		inv := r.Invites[len(r.Invites)-1]
		path = "/i/" + code + "/" + inv.Token
	})
	return
}

func TestInvitesAndChainOfCommand(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code

	// A commander invite: single use, skips approval.
	ref.post(base+"/invites", url.Values{"role": {"commander"}, "uses": {"1"}})
	cmdLink := newestInvite(store, code)
	cmd := newBrowser(t, ts)
	_, body := cmd.get(cmdLink)
	mustContain(t, body, "as a commander")
	_, body = cmd.post(cmdLink, url.Values{"name": {"Apone"}})
	mustContain(t, body, "Briefing")
	if m := memberByName(store, code, "Apone"); m.Role != game.RoleCommander || m.Status != game.StatusApproved {
		t.Fatalf("Apone = %s %s", m.Role, m.Status)
	}
	if res, body := newBrowser(t, ts).post(cmdLink, url.Values{"name": {"Gorman"}}); res.StatusCode != 400 || !strings.Contains(body, "used up") {
		t.Errorf("second use of a single-use invite: %d", res.StatusCode)
	}
	apone := memberByName(store, code, "Apone")

	// A player invite that reports to Apone, used twice.
	ref.post(base+"/invites", url.Values{"role": {"player"}, "uses": {"5"}, "reports_to": {apone.ID}})
	pLink := newestInvite(store, code)
	hicks, hudson := newBrowser(t, ts), newBrowser(t, ts)
	_, body = hicks.get(pLink)
	mustContain(t, body, "reporting to Apone")
	hicks.post(pLink, url.Values{"name": {"Hicks"}})
	hudson.post(pLink, url.Values{"name": {"Hudson"}})
	// A player outside Apone's command, joined the normal way.
	vasquez := newBrowser(t, ts)
	vasquez.post("/join", url.Values{"code": {code}, "name": {"Vasquez"}})
	v := memberByName(store, code, "Vasquez")
	ref.post(base+"/members/"+v.ID+"/approve", nil)
	h1, h2 := memberByName(store, code, "Hicks"), memberByName(store, code, "Hudson")
	if h1.ReportsTo != apone.ID || h2.ReportsTo != apone.ID || v.ReportsTo != "" {
		t.Fatal("reports-to not set from invite")
	}

	// The referee places m1 and m2 under Apone.
	for _, u := range []string{"m1", "m2"} {
		ref.post(base+"/units/"+u+"/assign", url.Values{"member": {apone.ID}})
	}
	assign := func(b *browser, unit, member string) int {
		res, _ := b.post(base+"/units/"+unit+"/assign", url.Values{"member": {member}})
		if res.StatusCode >= 300 && res.Request.Method == "GET" {
			return 200
		}
		return res.StatusCode
	}
	if c := assign(cmd, "m1", h1.ID); c != 200 {
		t.Errorf("commander assigns own unit to own player: %d", c)
	}
	if c := assign(cmd, "m1", h2.ID); c != 200 {
		t.Errorf("commander moves unit between own players: %d", c)
	}
	if c := assign(cmd, "m3", apone.ID); c != 400 {
		t.Errorf("commander takes a referee-held unit: %d", c)
	}
	if c := assign(cmd, "m2", v.ID); c != 400 {
		t.Errorf("commander gives a unit outside their command: %d", c)
	}
	if c := assign(cmd, "m2", ""); c != 400 {
		t.Errorf("commander returns a unit to the referee: %d", c)
	}
	if c := assign(hicks, "m1", h1.ID); c != 403 {
		t.Errorf("player assigns: %d", c)
	}
	store.Read(code, func(r *game.Room) {
		if r.Unit("m1").AssignedTo != h2.ID || r.Unit("m2").AssignedTo != apone.ID || r.Unit("m3").AssignedTo != "" {
			t.Errorf("assignments: m1=%s m2=%s m3=%s", r.Unit("m1").AssignedTo, r.Unit("m2").AssignedTo, r.Unit("m3").AssignedTo)
		}
	})

	// Only the referee manages invites and the chain.
	ref.post(base+"/start", nil)
	cmd.post(base+"/briefed", nil)
	_, body = cmd.room(base)
	mustContain(t, body, "Your command", "Hicks", "Hudson", "Give your units orders: 0 of 2 committed")
	mustNotContain(t, body, "/i/"+code, "Create invite link", "Promote to commander")
	if res, _ := cmd.post(base+"/invites", url.Values{"role": {"player"}, "uses": {"1"}}); res.StatusCode != 403 {
		t.Errorf("commander created an invite: %d", res.StatusCode)
	}
	if res, _ := cmd.post(base+"/members/"+v.ID+"/promote", nil); res.StatusCode != 403 {
		t.Errorf("commander promoted someone: %d", res.StatusCode)
	}

	// Referee moves Vasquez under Apone, promotes and demotes.
	ref.post(base+"/members/"+v.ID+"/reports-to", url.Values{"commander": {apone.ID}})
	if memberByName(store, code, "Vasquez").ReportsTo != apone.ID {
		t.Error("reports-to not changed")
	}
	ref.post(base+"/members/"+h1.ID+"/promote", nil)
	if m := memberByName(store, code, "Hicks"); m.Role != game.RoleCommander || m.ReportsTo != "" {
		t.Errorf("promoted Hicks = %s, reports to %q", m.Role, m.ReportsTo)
	}
	ref.post(base+"/members/"+apone.ID+"/demote", nil)
	if m := memberByName(store, code, "Hudson"); m.ReportsTo != "" {
		t.Error("demoting a commander left players reporting to them")
	}
	if c := assign(cmd, "m2", apone.ID); c != 403 {
		t.Errorf("demoted commander still assigns: %d", c)
	}

	// Revoked invites stop working.
	ref.post(base+"/invites", url.Values{"role": {"player"}, "uses": {"20"}})
	link := newestInvite(store, code)
	var id string
	store.Read(code, func(r *game.Room) { id = r.Invites[len(r.Invites)-1].ID })
	ref.post(base+"/invites/"+id+"/revoke", nil)
	_, body = newBrowser(t, ts).get(link)
	mustContain(t, body, "expired, been used up, or been revoked")
	if res, _ := newBrowser(t, ts).get("/i/" + code + "/deadbeef"); res.StatusCode != 404 {
		t.Errorf("bogus invite: %d", res.StatusCode)
	}
}

func TestInviteDoesNotStrandExistingSeat(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	ref.post("/r/"+code+"/invites", url.Values{"role": {"player"}, "uses": {"5"}})
	link := newestInvite(store, code)
	if res, _ := ref.post(link, url.Values{"name": {"Drake"}}); res.StatusCode != 409 {
		t.Errorf("accepting over an existing seat: %d", res.StatusCode)
	}
	res, _ := ref.post(link, url.Values{"name": {"Drake"}, "seat": {"tab"}})
	if !strings.Contains(res.Request.URL.Path, "/k/") {
		t.Errorf("tab seat not used: %s", res.Request.URL.Path)
	}
	_, body := ref.room("/r/" + code)
	mustContain(t, body, "Create invite link") // still the referee in this browser
}

func TestTurnClockRoutes(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	player := newBrowser(t, ts)
	player.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	ref.post(base+"/members/"+memberByName(store, code, "Hicks").ID+"/approve", nil)
	player.post(base+"/briefed", nil)

	res, body := ref.post(base+"/turn/schedule", url.Values{"tz": {"America/New_York"}, "time": {"18:00", "", "23:30", ""},
		"day": {"1", "2", "3", "4", "5"}, "takeover": {"12"}})
	if res.StatusCode != 200 {
		t.Fatalf("schedule: %d %s", res.StatusCode, body)
	}
	if res, body := ref.post(base+"/turn/schedule", url.Values{"tz": {"Mars/Olympus"}, "time": {"18:00"}, "day": {"1"}, "takeover": {"6"}}); res.StatusCode != 400 {
		t.Errorf("bad zone accepted: %d %s", res.StatusCode, body)
	}
	if res, _ := player.post(base+"/turn/lock", nil); res.StatusCode != 403 {
		t.Errorf("player locked the turn: %d", res.StatusCode)
	}
	ref.post(base+"/start", nil)
	var deadline time.Time
	store.Read(code, func(r *game.Room) { deadline = r.Deadline })
	if wd := deadline.In(mustZone(t, "America/New_York")); wd.Weekday() == time.Saturday || wd.Weekday() == time.Sunday {
		t.Errorf("weekday schedule locks on %s", wd.Weekday())
	}

	_, body = player.room(base)
	mustContain(t, body, "locks", "Orders lock")
	ref.post(base+"/turn/defer", url.Values{"preset": {"3"}})
	store.Read(code, func(r *game.Room) {
		if !r.Deferred || !r.Deadline.Equal(deadline.Add(3*time.Hour)) {
			t.Errorf("defer +3h: %s", r.Deadline)
		}
	})

	// The store's clock locks the turn once its deadline passes.
	store.Read(code, func(r *game.Room) { deadline = r.Deadline })
	store.Tick(deadline.Add(time.Second))
	_, body = player.room(base)
	mustContain(t, body, "Turn 1 is locked")
	_, body = ref.room(base)
	mustContain(t, body, "Turn 1 resolution", "Publish turn 1 and open turn 2")
	var unsettled int
	store.Read(code, func(r *game.Room) { unsettled = r.Unsettled() })
	if unsettled > 0 {
		if res, _ := ref.post(base+"/turn/next", nil); res.StatusCode != 400 {
			t.Errorf("published with fights unsettled: %d", res.StatusCode)
		}
	}
	ref.post(base+"/encounters/settle-all", nil)
	ref.post(base+"/turn/next", nil)
	store.Read(code, func(r *game.Room) {
		if r.Turn != 2 || r.Phase != game.PhasePlanning || len(r.Reports) != 1 {
			t.Errorf("after publishing: turn %d %s, %d reports", r.Turn, r.Phase, len(r.Reports))
		}
	})
	_, body = player.room(base)
	mustContain(t, body, "Turn 1:", `id="reports"`)
}

func TestSettlingAFight(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	h := memberByName(store, code, "Hicks")
	ref.post(base+"/members/"+h.ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)
	// A squad, civilians, and stragglers about to meet.
	store.Update(code, func(r *game.Room) error {
		r.Units = []*game.Unit{
			{ID: "m", Name: "Squad", Class: "rifle_squad", Side: "marines", Col: 22, Row: 2, Strength: 10, Known: true, AssignedTo: h.ID},
			{ID: "c", Name: "Hikers", Class: "civilian_group", Side: "civilians", Col: 22, Row: 2, Strength: 12, Known: true},
			{ID: "z", Name: "Stragglers", Class: "horde", Side: "infected", Col: 22, Row: 4, Strength: 30},
		}
		return nil
	})
	ref.post(base+"/turn/lock", nil)
	var encID string
	store.Read(code, func(r *game.Room) {
		if len(r.Resolution.Encounters) != 1 {
			t.Fatalf("encounters: %d", len(r.Resolution.Encounters))
		}
		encID = r.Resolution.Encounters[0].ID
	})
	_, body := ref.room(base)
	mustContain(t, body, "Fight at", "Suggested pool", "Roll suggested pools", "1 to settle")
	// Hits from the table make it a referee ruling.
	res, msg := ref.post(base+"/encounters/"+encID+"/settle", url.Values{"mode": {"hits"}, "dice_f": {"5"}, "dice_e": {"6"},
		"hits_f": {"4"}, "hits_e": {"1"}, "reason": {"Rolled at the table"}})
	if res.StatusCode != 200 {
		t.Fatalf("settle: %d %s", res.StatusCode, msg)
	}
	store.Read(code, func(r *game.Room) {
		e := r.Resolution.Encounters[0]
		if !e.Settled || e.Source != "referee" || e.Winner != "friend" || e.Margin != 3 || e.Reason != "Rolled at the table" {
			t.Errorf("encounter %+v", e)
		}
	})
	if res, _ := hicks.post(base+"/encounters/"+encID+"/settle", url.Values{"mode": {"rules"}}); res.StatusCode != 403 {
		t.Errorf("player settled a fight: %d", res.StatusCode)
	}
	ref.post(base+"/turn/next", nil)
	store.Read(code, func(r *game.Room) {
		z := r.Unit("z")
		if z == nil || z.Strength >= 30 {
			t.Errorf("the stragglers took no casualties: %+v", z)
		}
		if r.Stats.Won != 1 || r.Stats.Encounters != 1 {
			t.Errorf("stats %+v", r.Stats)
		}
	})
	_, body = hicks.room(base)
	mustContain(t, body, "Fight at", "We came out on top")
	mustNotContain(t, body, "Stragglers")
	_, body = ref.room(base)
	mustContain(t, body, "Stragglers (")
}

func mustZone(t *testing.T, name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestOrdersOverHTTPAndClosing(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	h := memberByName(store, code, "Hicks")
	ref.post(base+"/members/"+h.ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/units/m1/assign", url.Values{"member": {h.ID}})

	// No orders during setup.
	if res, _ := hicks.post(base+"/orders", url.Values{"unit": {"m1"}, "task": {"hold"}, "risk": {"balanced"}, "fallback": {"hold"}}); res.StatusCode != 400 {
		t.Errorf("order during setup: %d", res.StatusCode)
	}
	ref.post(base+"/start", nil)

	_, body := hicks.room(base)
	mustContain(t, body, "Orders, turn 1", "Give order", "Write an order", `"reach":{"m1":`)
	mustNotContain(t, body, `"m2":{"`) // only your own units' reach

	res, _ := hicks.post(base+"/orders", url.Values{"unit": {"m1"}, "task": {"move"}, "dest": {"0708"},
		"intent": {"Link up with the sappers"}, "risk": {"balanced"}, "fallback": {"hold"}, "submit": {"commit"}})
	if res.StatusCode != 200 {
		t.Fatalf("commit order: %d", res.StatusCode)
	}
	if res, _ := hicks.post(base+"/orders", url.Values{"unit": {"m2"}, "task": {"hold"}, "risk": {"balanced"}, "fallback": {"hold"}}); res.StatusCode != 403 {
		t.Errorf("ordered someone else's unit: %d", res.StatusCode)
	}
	_, body = hicks.room(base)
	mustContain(t, body, "committed", "move to 0708", `class="order-line s-committed mine"`)
	// The referee is blind to player orders during planning...
	_, body = ref.room(base)
	mustNotContain(t, body, "Link up with the sappers", "order-line", "move to 0708")
	mustContain(t, body, "Orders in: 1 of 1", "refereeing blind")
	if res, _ := ref.post(base+"/orders", url.Values{"unit": {"m1"}, "task": {"hold"}, "risk": {"balanced"}, "fallback": {"hold"}}); res.StatusCode != 400 {
		t.Errorf("blind referee overwrote a player's order: %d", res.StatusCode)
	}
	// ...until they switch to admin view, which players are told about.
	ref.post(base+"/admin-mode", url.Values{"on": {"1"}})
	_, body = ref.room(base)
	mustContain(t, body, "Link up with the sappers", `class="order-line s-committed`, "Admin view")
	_, body = hicks.room(base)
	mustContain(t, body, "The referee switched to admin view")
	ref.post(base+"/admin-mode", url.Values{"on": {"0"}})

	// Closing stops everything; players see the closed page; reopening restores.
	ref.post(base+"/close", nil)
	_, body = hicks.room(base)
	mustContain(t, body, "is closed")
	if res, _ := hicks.post(base+"/orders", url.Values{"unit": {"m1"}, "task": {"hold"}, "risk": {"balanced"}, "fallback": {"hold"}}); res.StatusCode != 400 {
		t.Errorf("order in a closed room: %d", res.StatusCode)
	}
	if res, _ := newBrowser(t, ts).post("/join", url.Values{"code": {code}, "name": {"Late"}}); res.StatusCode != 400 {
		t.Errorf("joined a closed room: %d", res.StatusCode)
	}
	if res, _ := hicks.post(base+"/reopen", nil); res.StatusCode != 403 {
		t.Errorf("player reopened: %d", res.StatusCode)
	}
	ref.post(base+"/reopen", nil)
	_, body = hicks.room(base)
	mustContain(t, body, "Orders, turn 1")

	// Deleting needs the room closed and the code typed.
	if res, _ := ref.post(base+"/delete", url.Values{"confirm": {code}}); res.StatusCode != 400 {
		t.Errorf("deleted an open room: %d", res.StatusCode)
	}
	ref.post(base+"/close", nil)
	if res, _ := ref.post(base+"/delete", url.Values{"confirm": {"WRONG1"}}); res.StatusCode != 400 {
		t.Errorf("deleted without the right code: %d", res.StatusCode)
	}
	ref.post(base+"/delete", url.Values{"confirm": {strings.ToLower(code)}})
	if err := store.Read(code, func(*game.Room) {}); err == nil {
		t.Error("room still exists after delete")
	}
}

func TestRulesCardsAndLiveOrderForm(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	h := memberByName(store, code, "Hicks")
	ref.post(base+"/members/"+h.ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)

	// No units yet: the order form and panel are still in the page, hidden,
	// so a live update can fill them when units are assigned.
	_, body := hicks.room(base)
	mustContain(t, body, `id="order-box"`, `id="orders-panel"`)
	ref.post(base+"/units/m1/assign", url.Values{"member": {h.ID}})
	_, body = hicks.room(base)
	mustContain(t, body, `"units":[{"id":"m1"`)

	_, body = hicks.get(base + "/rules")
	mustContain(t, body, "How to play", "No order, no attack", "Stacking", "Marine rifle squad", "Experience")
	mustNotContain(t, body, "Brood Mother", "Referee notes")
	_, body = ref.get(base + "/rules")
	mustContain(t, body, "Brood Mother", "Referee notes")

	_, body = hicks.get(base + "/units/m1")
	mustContain(t, body, `id="unit-card"`, "1st Rifle Squad", "Movement", "Experience: Green")
	if res, _ := hicks.get(base + "/units/z1"); res.StatusCode != 404 {
		t.Errorf("player read an enemy card: %d", res.StatusCode)
	}
	if res, _ := newBrowser(t, ts).get(base + "/rules"); res.StatusCode != 403 {
		t.Errorf("stranger read the rules: %d", res.StatusCode)
	}

	ref.post(base+"/operation/end", nil)
	_, body = ref.get(base + "/units/m1")
	mustContain(t, body, "Experience: Seasoned", "Operation 1")
}

func TestRefereeSeesWhenAllOrdersAreIn(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	h := memberByName(store, code, "Hicks")
	ref.post(base+"/members/"+h.ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/units/m1/assign", url.Values{"member": {h.ID}})
	ref.post(base+"/units/m3/assign", url.Values{"member": {h.ID}})
	ref.post(base+"/start", nil)

	order := func(unit string) {
		res, body := hicks.post(base+"/orders", url.Values{"unit": {unit}, "task": {"hold"}, "risk": {"balanced"}, "fallback": {"hold"}, "submit": {"commit"}})
		if res.StatusCode != 200 {
			t.Fatalf("order %s: %d %s", unit, res.StatusCode, body)
		}
	}
	order("m1")
	_, body := ref.room(base)
	mustContain(t, body, "Orders in: 1 of 2", "Waiting on Hicks (1 unit)", "1 of 2 player units have committed orders")
	mustNotContain(t, body, "Every player has committed")

	order("m3")
	_, body = ref.room(base)
	mustContain(t, body, "Every player has committed orders (2 of 2 units)", ">Lock turn 1 now</button>", "Everyone is done")
	_, body = hicks.room(base)
	mustNotContain(t, body, "Lock turn 1 now")

	ref.post(base+"/turn/lock", nil)
	store.Read(code, func(r *game.Room) {
		if r.Phase != game.PhaseResolving {
			t.Errorf("phase %s after manual lock", r.Phase)
		}
	})
}

func TestScreensByRole(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	ref.post(base+"/members/"+memberByName(store, code, "Hicks").ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)

	noRedirect := &http.Client{Jar: hicks.c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, screen := range []string{"turn", "events", "room"} {
		res, err := noRedirect.Get(ts.URL + base + "/" + screen)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			t.Errorf("player opened the referee's %s screen: %d", screen, res.StatusCode)
		}
	}
	for _, screen := range []string{"turn", "events", "forces", "people", "reports", "room"} {
		res, body := ref.get(base + "/" + screen)
		if res.StatusCode != 200 || !strings.Contains(body, `aria-current="page"`) || !strings.Contains(body, `id="status-cells"`) {
			t.Errorf("referee %s screen: %d", screen, res.StatusCode)
		}
	}
	_, body := hicks.get(base + "/orders")
	mustContain(t, body, "Orders, turn 1", ">Orders", ">Forces", ">Reports")
	mustNotContain(t, body, ">Events<", ">Room<", "Editor")
}

func TestLiveStreamIsNotCompressed(t *testing.T) {
	ts, _ := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	req, _ := http.NewRequest("GET", ts.URL+"/r/"+code+"/live", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := ref.c.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" || res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("live stream: %q, encoding %q", ct, res.Header.Get("Content-Encoding"))
	}
	buf := make([]byte, 64)
	n, _ := res.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: hello") {
		t.Errorf("first bytes %q", buf[:n])
	}
}
