package web

import (
	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
	"net/url"
	"testing"
	"time"
)

func TestAutomaticRefereeControls(t *testing.T) {
	ts, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	path := base + "/assistant/automatic"
	form := url.Values{"action": {"enable"}}
	if res, _ := ref.post(path, form); res.StatusCode != 400 {
		t.Fatal("enabled without model")
	}
	ts.Config.Handler.(*Server).SetAI(ai.New(ai.Config{BaseURL: "http://127.0.0.1:1", Model: "test-referee"}))
	if res, _ := player.post(path, form); res.StatusCode != 403 {
		t.Fatal("player could enable referee")
	}
	_, body := ref.get(base + "/setup")
	mustContain(t, body, "Enable automatic referee")
	if res, body := ref.post(path, form); res.StatusCode != 200 {
		t.Fatalf("enable: %d %s", res.StatusCode, body)
	}
	_, body = ref.get(base + "/setup")
	mustContain(t, body, "Automatic referee is on.", "Pause AI")
	_, body = player.room(base)
	mustNotContain(t, body, "Enable automatic referee", "Pause AI", "test-referee")
	ref.post(base+"/start", nil)
	store.Update(code, func(r *game.Room) error {
		r.Tick(store.Cat.Modules[r.ModuleID], r.Deadline)
		run := r.NextRun("test-referee", "test", time.Now())
		r.FinishRun(run.ID, 0, game.ErrBadRequest, nil, false, time.Now())
		return nil
	})
	_, body = ref.get(base + "/desk")
	mustContain(t, body, "Adjudication stopped.", "Retry adjudication")
	_, body = player.get(base)
	mustContain(t, body, "Adjudication needs the referee", "no partial result has been published")
	mustNotContain(t, body, "Retry adjudication", "test-referee", "bad request")
	if res, _ := ref.post(path, url.Values{"action": {"retry"}}); res.StatusCode != 200 {
		t.Fatal("retry failed")
	}
	if res, _ := ref.post(path, url.Values{"action": {"pause"}}); res.StatusCode != 200 {
		t.Fatal("pause failed")
	}
	store.Read(code, func(r *game.Room) {
		if r.Assistant.Automatic || r.HasPendingRun() {
			t.Error("pause did not cancel")
		}
	})
}
