package assist

import (
	"context"
	"encoding/json"
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

func TestWorkerRunsAutomaticTurnEndToEnd(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model     string
			Messages  []struct{ Role, Content string }
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Model != "referee-model" || req.MaxTokens == 0 {
			t.Error("wrong model or missing output budget")
		}
		calls++
		system, user := req.Messages[0].Content, req.Messages[1].Content
		reply := ""
		switch {
		case strings.Contains(system, `"intentions"`):
			var in struct{ Enemies []struct{ ID string } }
			json.Unmarshal([]byte(user), &in)
			var intentions []map[string]string
			for _, e := range in.Enemies {
				intentions = append(intentions, map[string]string{"unit": e.ID, "intent": "Hold position."})
			}
			b, _ := json.Marshal(map[string]any{"intentions": intentions})
			reply = string(b)
			if strings.Contains(user, "Watch the bridge") {
				t.Error("enemy planner saw player orders")
			}
		case strings.Contains(system, `"contests"`):
			reply = `{"contests":[]}`
		case strings.Contains(system, `"outcomes"`):
			var in struct{ Orders []struct{ ID string } }
			json.Unmarshal([]byte(user), &in)
			var outcomes []map[string]string
			for _, o := range in.Orders {
				outcomes = append(outcomes, map[string]string{"order": o.ID, "title": "Watch", "text": "The bridge remained quiet.", "seen": "all"})
			}
			b, _ := json.Marshal(map[string]any{"outcomes": outcomes, "units": []any{}, "reason": "No contacts reached the bridge."})
			reply = string(b)
		default:
			reply = "The bridge remained quiet. The squad awaits orders."
			if strings.Contains(user, "refereeNotes") || strings.Contains(user, "hiddenFromPlayers") {
				t.Error("briefing saw hidden context")
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	defer api.Close()
	cat, _ := game.LoadCatalog(content.FS)
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := game.OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.Create("AI turn", "st-ines", "Ref", game.HashToken("ref"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Update(code, func(r *game.Room) error {
		mod, ref := cat.Modules[r.ModuleID], r.Referee()
		now := time.Now()
		if err := r.StartOperation(ref, mod, now); err != nil {
			return err
		}
		if _, err := r.SaveOrder(ref, mod, "f-lantern", "Watch the bridge.", nil, true, now); err != nil {
			return err
		}
		if err := r.SetAutomatic(ref, "enable", now); err != nil {
			return err
		}
		r.Tick(mod, r.Deadline)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: store, Client: ai.New(ai.Config{BaseURL: api.URL, Model: "referee-model"})}
	for i := 0; i < 4; i++ {
		if !worker.Once(context.Background()) {
			t.Fatalf("stage %d absent", i)
		}
	}
	if worker.Once(context.Background()) || calls != 4 {
		t.Fatalf("unbounded work: %d calls", calls)
	}
	reopened, err := game.OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Read(code, func(r *game.Room) {
		if r.Turn != 2 || r.Phase != game.PhasePlanning || len(r.Reports) != 1 || len(r.Reports[0].Entries) != 1 || r.Reports[0].NarrativeStatus != game.NarrativeApproved {
			t.Errorf("automatic turn was not fully persisted")
		}
		for _, run := range r.Assistant.Runs {
			if run.Status != game.RunDone {
				t.Errorf("run failed: %+v", run)
			}
		}
	})
}
