package web

import (
	"asyncwargame/internal/game"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWaywardPagesAndMissingAI(t *testing.T) {
	ts, store := newTestServer(t)
	host := newBrowser(t, ts)
	res, body := host.post("/rooms", url.Values{"campaign": {"Tollhouse"}, "scenario": {"wayward-tollhouse"}, "name": {"Host"}})
	if res.StatusCode != 200 {
		t.Fatalf("create: %d %s", res.StatusCode, body)
	}
	code := strings.TrimPrefix(res.Request.URL.Path, "/r/")
	base := "/r/" + code
	res, body = host.post(base+"/start", nil)
	if res.StatusCode != 400 {
		t.Fatalf("must require AI config: %d", res.StatusCode)
	}
	mustContain(t, body, "Configure the AI API and model")
	// Exercise role-filtered pages using a real approved member, without making
	// any model requests in HTTP tests.
	player := newBrowser(t, ts)
	player.post("/join", url.Values{"code": {code}, "name": {"Player"}})
	if err := store.Update(code, func(r *game.Room) error {
		for _, m := range r.Members {
			if m.Name == "Player" {
				if err := r.Decide(r.Referee(), m.ID, "approve"); err != nil {
					return err
				}
				m.Briefed = true
				if err := r.Assign(r.Referee(), store.Cat.Modules[r.ModuleID], "mara", m.ID); err != nil {
					return err
				}
			}
		}
		return r.StartOperation(r.Referee(), store.Cat.Modules[r.ModuleID], time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	res, body = player.get(base)
	if res.StatusCode != 200 {
		t.Fatalf("player page: %d", res.StatusCode)
	}
	mustContain(t, body, "asynchronous exploration playtest", "Actions this exchange", "Character", "2d6", "Stamina")
	mustNotContain(t, body, "winter grain was", "grain was distributed as relief", "She wants assurance", "d10", "Usual bonus")
	res, body = player.get(base + "/units/mara")
	if res.StatusCode != 200 {
		t.Fatalf("character page: %d", res.StatusCode)
	}
	mustContain(t, body, "Wayward character sheet", "Merchant", "17 / 17", "Connection slots", "Social")
	mustNotContain(t, body, "d10", "Usual bonus", "Change this unit")
}
