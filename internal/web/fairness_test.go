package web

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"asyncwargame/internal/game"
)

func TestDeadlineEnforcedAndExplainedBeforeClockTick(t *testing.T) {
	_, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	if res, body := player.post(base+"/orders", url.Values{"unit": {"m2"}, "text": {"Original order"}, "submit": {"commit"}}); res.StatusCode != 200 {
		t.Fatalf("initial order: %d %s", res.StatusCode, body)
	}
	if err := store.Update(code, func(r *game.Room) error { r.Deadline = time.Now().Add(-time.Second); return nil }); err != nil {
		t.Fatal(err)
	}
	res, body := player.post(base+"/orders", url.Values{"unit": {"m2"}, "text": {"Late replacement"}, "submit": {"commit"}})
	if res.StatusCode != 400 {
		t.Fatalf("late order: %d %s", res.StatusCode, body)
	}
	mustContain(t, body, "deadline has passed")
	_, body = player.get(base)
	mustContain(t, body, "Orders are closed")
	store.Read(code, func(r *game.Room) {
		if r.CurrentOrder("m2").Text != "Original order" {
			t.Error("late request changed canonical order")
		}
	})
}

func TestPlayerOrderMarkupIsEscaped(t *testing.T) {
	_, _, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	text := `<img src=x onerror="document.body.remove()"> Hold position.`
	if res, body := player.post(base+"/orders", url.Values{"unit": {"m2"}, "text": {text}, "submit": {"commit"}}); res.StatusCode != 200 {
		t.Fatalf("save: %d %s", res.StatusCode, body)
	}
	_, body := player.get(base)
	if strings.Contains(body, `<img src=x`) {
		t.Fatal("order rendered as executable markup")
	}
	mustContain(t, body, "document.body.remove()")
	if !strings.Contains(body, "&lt;img") && !strings.Contains(body, `\u003cimg`) {
		t.Fatal("order was not safely encoded in either HTML or map JSON")
	}
}
