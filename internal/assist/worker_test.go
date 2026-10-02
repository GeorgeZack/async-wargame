package assist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"asyncwargame/content"
	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

func TestWorkerSuggestsRollsDraftsOutcomesAndNarrative(t *testing.T) {
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		system, user := req.Messages[0].Content, req.Messages[1].Content
		prompts = append(prompts, user)
		var orderID string
		var in struct {
			Orders []struct{ Order string } `json:"orders"`
		}
		json.Unmarshal([]byte(user), &in)
		if len(in.Orders) > 0 {
			orderID = in.Orders[0].Order
		}
		reply := "Okafor's grenade lands true; the lobby goes quiet."
		switch {
		case strings.Contains(system, `"contests"`):
			reply = "```json\n" + `{"contests":[{"order":"` + orderID + `","title":"The grenade","a":{"unit":"m2","factors":[{"name":"surprise","value":3}]},"b":{"unit":"z3","factors":[]}},` +
				`{"order":"nope","title":"bogus","a":{"unit":"m2"}},{"order":"` + orderID + `","title":"ghost","a":{"unit":"ghost"}}]}` + "\n```"
		case strings.Contains(system, "radio operator"):
			reply = "Grenade in the lobby. Quiet after."
		case strings.Contains(system, `"intentions"`):
			reply = `{"intentions":[{"unit":"z1","intent":"Follow the radio noise toward the beach."}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20}})
	}))
	defer srv.Close()

	cat, err := game.LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	store, err := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.Create("Lantern", "st-ines", "Ref", game.HashToken("ref"))
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{Store: store, Client: ai.New(ai.Config{BaseURL: srv.URL + "/v1", Model: "big-pickle", Name: "test"})}
	ctx := context.Background()
	now := time.Now()
	update := func(fn func(r *game.Room, mod *game.Module, ref *game.Member) error) {
		t.Helper()
		if err := store.Update(code, func(r *game.Room) error { return fn(r, cat.Modules[r.ModuleID], r.Referee()) }); err != nil {
			t.Fatal(err)
		}
	}
	var orderID string
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		if err := r.StartOperation(ref, mod, now); err != nil {
			return err
		}
		o, err := r.SaveOrder(ref, mod, "m2", "Okafor tosses a grenade into the lobby.", nil, true, now)
		if err != nil {
			return err
		}
		orderID = o.ID
		if err := r.SetAssistant(ref, true, true, true); err != nil {
			return err
		}
		return r.LockNow(ref, mod, now)
	})

	// At the lock: suggestions, unrolled.
	if !w.Once(ctx) {
		t.Fatal("no run after lock")
	}
	if !strings.Contains(prompts[0], "tosses a grenade") || !strings.Contains(prompts[0], `"id": "m2"`) {
		t.Fatalf("suggest prompt: %s", prompts[0])
	}
	var contests []game.Contest
	store.Read(code, func(r *game.Room) {
		for _, c := range r.Desk.Contests {
			contests = append(contests, *c)
		}
	})
	if len(contests) != 1 || contests[0].Source != "ai" || contests[0].Result != nil || contests[0].A.Bonus != 5 ||
		contests[0].A.Why != "quality +2, surprise +3" || contests[0].B == nil || contests[0].B.Bonus != 2 {
		t.Fatalf("suggested contests: %+v", contests)
	}

	// Roll it, then have the assistant draft the outcome.
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		if err := r.RollContest(ref, contests[0].ID, 6, 3, now); err != nil {
			return err
		}
		return r.RunNow(ref, game.RunOutcome, orderID, now)
	})
	if !w.Once(ctx) {
		t.Fatal("no outcome run")
	}
	if !strings.Contains(prompts[1], "The grenade") || !strings.Contains(prompts[1], "LCpl. Okafor 11 (rolled 6") {
		t.Errorf("outcome prompt lacks the roll: %s", prompts[1])
	}
	var oc game.Outcome
	store.Read(code, func(r *game.Room) { oc = *r.Desk.Outcomes[0] })
	if oc.Text != "Okafor's grenade lands true; the lobby goes quiet." || oc.Source != "ai" || oc.Model != "big-pickle" {
		t.Fatalf("outcome %+v", oc)
	}
	// Once the referee rewrites it, a new draft goes beside it, not over it.
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		if err := r.SetOutcome(ref, orderID, "", "", "My own words.", game.SeenByAll); err != nil {
			return err
		}
		return r.RunNow(ref, game.RunOutcome, orderID, now)
	})
	w.Once(ctx)
	store.Read(code, func(r *game.Room) { oc = *r.Desk.Outcomes[0] })
	if oc.Text != "My own words." || oc.Draft == "" {
		t.Errorf("referee text overwritten: %+v", oc)
	}

	// Publishing queues a narrative from the players' lines only.
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error { return r.PublishTurn(ref, mod, now) })
	if !w.Once(ctx) {
		t.Fatal("no narrative run")
	}
	var rep game.TurnReport
	store.Read(code, func(r *game.Room) { rep = *r.Reports[0] })
	if rep.Narrative != "Grenade in the lobby. Quiet after." || rep.NarrativeStatus != game.NarrativeDraft {
		t.Fatalf("narrative %q %s", rep.Narrative, rep.NarrativeStatus)
	}
	if !strings.Contains(prompts[len(prompts)-1], "My own words.") {
		t.Errorf("narrative prompt: %s", prompts[len(prompts)-1])
	}
	// Turn 2's planning: the assistant proposes what the enemy intends.
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		return r.RunNow(ref, game.RunIntentions, "", now)
	})
	if !w.Once(ctx) {
		t.Fatal("no intentions run")
	}
	store.Read(code, func(r *game.Room) {
		if r.IntentionOf("z1") != "Follow the radio noise toward the beach." {
			t.Errorf("intention %q", r.IntentionOf("z1"))
		}
	})
	if w.Once(ctx) {
		t.Error("worked with nothing queued")
	}
}

func TestCancellingCutsOffTheModel(t *testing.T) {
	started := make(chan struct{}, 1)
	quit := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select { // a slow model: answers after a minute, unless the caller hangs up
		case <-r.Context().Done():
			return
		case <-quit:
			return
		case <-time.After(time.Minute):
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"contests":[]}`}}}})
	}))
	defer srv.Close()
	defer close(quit) // runs first, so Close doesn't wait on the slow handler
	cat, _ := game.LoadCatalog(content.FS)
	store, _ := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	code, _ := store.Create("Lantern", "st-ines", "Ref", game.HashToken("ref"))
	w := &Worker{Store: store, Client: ai.New(ai.Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "test"})}
	now := time.Now()
	store.Update(code, func(r *game.Room) error {
		mod, ref := cat.Modules[r.ModuleID], r.Referee()
		r.StartOperation(ref, mod, now)
		r.SaveOrder(ref, mod, "m2", "Okafor tosses a grenade.", nil, true, now)
		r.SetAssistant(ref, true, true, false)
		return r.LockNow(ref, mod, now) // queues the roll suggestions
	})

	done := make(chan time.Duration)
	go func() {
		t0 := time.Now()
		w.Once(context.Background())
		done <- time.Since(t0)
	}()
	<-started
	if err := store.Update(code, func(r *game.Room) error {
		_, err := r.CancelRuns(r.Referee(), "", "suggest", "", false, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case took := <-done:
		if took > 10*time.Second {
			t.Errorf("the call took %v after cancelling", took)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancelling didn't stop the model call")
	}
	store.Read(code, func(r *game.Room) {
		run := r.Assistant.Runs[len(r.Assistant.Runs)-1]
		if run.Status != game.RunCancelled || run.Error != "" {
			t.Errorf("run %s %q", run.Status, run.Error)
		}
		if r.HasPendingRun() {
			t.Error("the cancelled run went back in the queue")
		}
	})
	// Nothing left to cancel says so; a player can't cancel.
	store.Update(code, func(r *game.Room) error {
		if _, err := r.CancelRuns(r.Referee(), "", "", "", true, time.Now()); err == nil {
			t.Error("cancelled nothing without saying so")
		}
		if _, err := r.CancelRuns(&game.Member{Role: game.RolePlayer}, "", "", "", true, time.Now()); err != game.ErrForbidden {
			t.Errorf("a player cancelled: %v", err)
		}
		return nil
	})
}
