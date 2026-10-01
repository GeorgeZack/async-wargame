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

func TestWorkerReviewsOrdersAndDraftsNarrative(t *testing.T) {
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []struct{ Role, Content string } }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		system, user := req.Messages[0].Content, req.Messages[1].Content
		prompts = append(prompts, user)
		reply := `The bridge is the 1st Rifle Squad's problem.`
		if strings.Contains(system, `"issues"`) {
			reply = "```json\n" + `{"issues":[{"unit":"m1","severity":"judgment","issue":"The intent says hold the bridge but the squad is sent past it.","suggestion":"Ask what the player meant."},{"unit":"ghost","severity":"weird","issue":"A unit that doesn't exist."}]}` + "\n```"
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
	code, err := store.Create("Lantern", "dark-coast-rescue", "Ref", game.HashToken("ref"))
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{Store: store, Client: ai.New(ai.Config{BaseURL: srv.URL + "/v1", Model: "big-pickle", Name: "test"})}
	ctx := context.Background()
	now := time.Now()

	// The referee orders the 1st Rifle Squad somewhere, switches the assistant on, and locks.
	update := func(fn func(r *game.Room, mod *game.Module, ref *game.Member) error) {
		t.Helper()
		if err := store.Update(code, func(r *game.Room) error { return fn(r, cat.Modules[r.ModuleID], r.Referee()) }); err != nil {
			t.Fatal(err)
		}
	}
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		if err := r.StartOperation(ref, mod, now); err != nil {
			return err
		}
		u := r.Unit("m1")
		var dest string
		for h, st := range r.Reach(mod, u) {
			if st.Cost > 0 {
				dest = game.HexID(h[0], h[1])
				break
			}
		}
		if _, err := r.SaveOrder(ref, mod, game.OrderInput{Unit: "m1", Task: "move", Dest: dest, Intent: "Hold the bridge", Risk: "balanced", Fallback: "hold", Commit: true}, now); err != nil {
			return err
		}
		if err := r.SetAssistant(ref, true, true, true); err != nil {
			return err
		}
		return r.LockNow(ref, mod, now)
	})
	if !w.Once(ctx) {
		t.Fatal("no run picked up after lock")
	}
	if len(prompts) != 1 || !strings.Contains(prompts[0], `"unit": "m1"`) || !strings.Contains(prompts[0], "Hold the bridge") {
		t.Fatalf("review prompt: %s", prompts)
	}
	var issues []*game.Issue
	var run game.Run
	store.Read(code, func(r *game.Room) {
		for _, is := range r.Assistant.Issues {
			if is.Source == "ai" {
				issues = append(issues, is)
			}
		}
		run = *r.Assistant.Runs[0]
	})
	if len(issues) != 2 || issues[0].Unit != "m1" || issues[0].UnitName != "1st Rifle Squad" || issues[0].Model != "big-pickle" ||
		issues[1].Unit != "" || issues[1].Severity != game.SeverityInfo {
		t.Fatalf("issues: %+v %+v", issues[0], issues[1])
	}
	if run.Status != game.RunDone || run.Tokens != 120 || run.Prompt == "" || run.Provider != "test" {
		t.Errorf("run %+v", run)
	}

	// Publishing queues a narrative, drafted only from the players' lines.
	update(func(r *game.Room, mod *game.Module, ref *game.Member) error {
		if err := r.SettleAll(ref, mod); err != nil {
			return err
		}
		return r.PublishTurn(ref, mod, now)
	})
	if !w.Once(ctx) {
		t.Fatal("no narrative run")
	}
	var rep game.TurnReport
	store.Read(code, func(r *game.Room) { rep = *r.Reports[0] })
	if rep.Narrative != "The bridge is the 1st Rifle Squad's problem." || rep.NarrativeStatus != game.NarrativeDraft {
		t.Fatalf("narrative %q %s", rep.Narrative, rep.NarrativeStatus)
	}
	for _, line := range rep.Referee {
		if !contains(rep.Players, line) && strings.Contains(prompts[1], line) {
			t.Errorf("a referee-only line reached the narrative prompt: %q", line)
		}
	}
	if w.Once(ctx) {
		t.Error("worked with nothing queued")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
