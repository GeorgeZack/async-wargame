package playtest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"asyncwargame/content"
	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

type fakeModel struct {
	mu      sync.Mutex
	calls   int
	invalid bool
	combat  bool
}

func (*fakeModel) Model() string    { return "deterministic-test" }
func (*fakeModel) Provider() string { return "test" }
func (f *fakeModel) Complete(ctx context.Context, system, user string, _ int) (string, ai.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return "", ai.Usage{}, e
	}
	f.calls++
	var out any
	switch {
	case strings.Contains(system, "one player"):
		var in struct {
			AllowedOrders []game.OrderUnit `json:"allowedOrders"`
		}
		json.Unmarshal([]byte(user), &in)
		orders := []any{}
		// Commander writes a global order; half the subordinate seats override it.
		if len(in.AllowedOrders) > 0 {
			u := in.AllowedOrders[0]
			for _, candidate := range in.AllowedOrders {
				if strings.HasPrefix(candidate.ID, "f-") {
					u = candidate
				}
			}
			if f.calls%2 == 0 || strings.HasPrefix(u.ID, "f-") || strings.HasPrefix(u.ID, "p") {
				orders = append(orders, map[string]any{"unit": u.ID, "text": "Hold position and cover the other detachment."})
			}
		}
		out = map[string]any{"orders": orders}
	case strings.Contains(system, `"intentions"`):
		var in struct{ Enemies []struct{ ID string } }
		json.Unmarshal([]byte(user), &in)
		intents := []any{}
		for _, e := range in.Enemies {
			intents = append(intents, map[string]string{"unit": e.ID, "intent": "Hold and observe."})
		}
		out = map[string]any{"intentions": intents}
	case strings.Contains(system, `"contests"`):
		var in struct {
			Orders []struct {
				ID      string
				Members []struct{ ID string }
			}
		}
		json.Unmarshal([]byte(user), &in)
		contests := []any{}
		if f.combat {
			for _, o := range in.Orders {
				if len(o.Members) > 0 {
					contests = append(contests, map[string]any{"order": o.ID, "title": "Contact under pressure", "a": map[string]string{"unit": o.Members[0].ID}})
				}
			}
		}
		out = map[string]any{"contests": contests}
	case strings.Contains(system, `"outcomes"`):
		var in struct {
			Orders []struct{ ID string }
			Units  []struct {
				ID        string
				Destroyed bool
			}
			Turn int
		}
		json.Unmarshal([]byte(user), &in)
		if f.invalid {
			f.invalid = false
			return `{"outcomes":[],"reason":"missing orders"}`, ai.Usage{}, nil
		}
		outcomes := []any{}
		for _, o := range in.Orders {
			outcomes = append(outcomes, map[string]string{"order": o.ID, "title": "Holding", "text": "The unit held position.", "seen": "all"})
		}
		units := []any{}
		if in.Turn == 1 {
			for _, u := range in.Units {
				if u.ID == "z1" || u.ID == "east" || f.combat && u.ID == "e001" {
					units = append(units, map[string]any{"unit": u.ID, "strength": 0, "status": "Destroyed during the test engagement"})
				}
			}
		}
		out = map[string]any{"outcomes": outcomes, "units": units, "reason": "Deterministic stress fixture exercises publication, casualties and retries."}
	default:
		return "The detachments held position.", ai.Usage{Prompt: 2, Completion: 3}, nil
	}
	b, _ := json.Marshal(out)
	return string(b), ai.Usage{Prompt: 2, Completion: 3}, nil
}
func TestRepeatedCampaignsAndValidationRetry(t *testing.T) {
	cat, e := game.LoadCatalog(content.FS)
	if e != nil {
		t.Fatal(e)
	}
	for _, scenario := range []string{"st-ines", "crossroads"} {
		t.Run(scenario, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "run")
			s, e := Run(context.Background(), cat, &fakeModel{invalid: true}, Config{Directory: dir, Scenario: scenario, Games: 2, Turns: 12, MaxCalls: 300, Contact: true})
			if e != nil {
				t.Fatal(e)
			}
			expected := 12
			if scenario == "crossroads" {
				expected = 6
			}
			if len(s.Games) != 2 || s.Games[0].Turns != expected || s.Games[1].Turns != expected {
				t.Fatalf("incomplete campaigns: %+v", s)
			}
			if len(s.Games[0].Failures) != 1 || len(s.Games[0].Destroyed) != 1 {
				t.Fatalf("missing regression exercise: %+v", s.Games[0])
			}
			reopened, e := game.OpenStore(filepath.Join(dir, "campaigns.json"), cat)
			if e != nil {
				t.Fatal(e)
			}
			for _, code := range reopened.Codes() {
				reopened.Read(code, func(r *game.Room) {
					want := expected + 1
					if r.MissionComplete() {
						want = expected
					}
					if r.Turn != want || r.HasPendingRun() {
						t.Error("published state did not survive restart")
					}
				})
			}
			raw, e := os.ReadFile(filepath.Join(dir, "summary.json"))
			if e != nil {
				t.Fatal(e)
			}
			var saved Summary
			if json.Unmarshal(raw, &saved) != nil || saved.Calls != s.Calls || saved.Finished.IsZero() {
				t.Fatal("final summary not saved")
			}
		})
	}
}
func TestBudgetsAndExistingDirectory(t *testing.T) {
	cat, _ := game.LoadCatalog(content.FS)
	dir := filepath.Join(t.TempDir(), "run")
	f := &fakeModel{}
	s, e := Run(context.Background(), cat, f, Config{Directory: dir, Games: 2, Turns: 2, MaxCalls: 1})
	if e == nil || f.calls != 1 || s.Calls != 1 {
		t.Fatalf("call limit failed: %d %v", f.calls, e)
	}
	if _, e := os.Stat(filepath.Join(dir, "summary.json")); e != nil {
		t.Fatal("failure lost summary")
	}
	if _, e = Run(context.Background(), cat, f, Config{Directory: dir, Games: 1, Turns: 1, MaxCalls: 10}); e == nil || f.calls != 1 {
		t.Fatal("existing run was overwritten")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = Run(ctx, cat, f, Config{Directory: filepath.Join(t.TempDir(), "cancelled"), Games: 1, Turns: 1, MaxCalls: 10})
	if e == nil || f.calls != 1 {
		t.Fatal("cancelled context still called provider")
	}
}
func TestPlayerPromptsKeepRefereeAndOtherPlayerSecretsOut(t *testing.T) {
	cat, _ := game.LoadCatalog(content.FS)
	store, _ := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	code, seats, e := setup(store, "crossroads", false)
	if e != nil {
		t.Fatal(e)
	}
	if len(seats) != 2 {
		t.Fatal("two-player scenario did not create two seats")
	}
	store.Update(code, func(r *game.Room) error {
		r.RefereeNotes = "REFEREE-SECRET"
		r.Unit("wood").Name = "ENEMY-SECRET"
		r.Unit("scouts").Brief = "SECOND-PLAYER-SECRET"
		return nil
	})
	prompt, e := playerPrompt(store, code, seats[0])
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"REFEREE-SECRET", "ENEMY-SECRET", "SECOND-PLAYER-SECRET"} {
		if strings.Contains(prompt, secret) {
			t.Fatal("player sees " + secret)
		}
	}
	prompt, e = playerPrompt(store, code, seats[1])
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(prompt, "SECOND-PLAYER-SECRET") {
		t.Fatal("own briefing omitted")
	}
}
func TestRecorderFailsBeforeExtraCalls(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "calls")
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	rec := &recorder{Client: &fakeModel{}, file: f, max: 2}
	_, _, e = rec.Complete(context.Background(), "narrate", "", 10)
	if e == nil {
		t.Fatal("record write error ignored")
	}
	_, _, again := rec.Complete(context.Background(), "narrate", "", 10)
	if !errors.Is(e, again) || rec.calls != 1 {
		t.Fatal("kept spending with broken recording")
	}
}

func TestHundredPlayersBatchedJudgeAndRestart(t *testing.T) {
	cat, err := game.LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	turns := 3
	if raw := os.Getenv("AW_SCALE_SOAK_TURNS"); raw != "" {
		var e error
		turns, e = strconv.Atoi(raw)
		if e != nil || turns < 3 || turns > 12 {
			t.Fatal("AW_SCALE_SOAK_TURNS must be 3–12")
		}
	}
	dir := filepath.Join(t.TempDir(), "hundred")
	s, err := Run(context.Background(), cat, &fakeModel{invalid: true, combat: true}, Config{Directory: dir, Scenario: "crossroads", Players: 100, Concurrency: 8, Games: 1, Turns: turns, MaxCalls: turns*150 + 1})
	if err != nil {
		t.Fatal(err)
	}
	if s.Games[0].Turns != turns || len(s.Games[0].Failures) != 1 || len(s.Games[0].Timings) != turns {
		t.Fatalf("incomplete scale run: %+v", s)
	}
	restoredCat, err := game.LoadCatalog(content.FS, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := game.OpenStore(filepath.Join(dir, "campaigns.json"), restoredCat)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Read(s.Games[0].Code, func(r *game.Room) {
		if len(r.Members) != 101 || len(r.Units) != 110 || r.Assistant.Batches != nil || r.HasPendingRun() {
			t.Error("load state lost seats or left queued work")
		}
		if len(s.Games[0].Destroyed) != 1 {
			t.Error("load fixture did not exercise casualties")
		}
		if len(r.Reports) != turns {
			t.Error("missing reports")
		}
		for _, rep := range r.Reports {
			if len(rep.Entries) != 100 {
				t.Errorf("missing/duplicate order outcomes: %d", len(rep.Entries))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("100 players, %d turns: %d completions; timing %+v; prompts %+v", turns, s.Calls, s.Games[0].Timings, s.Stages)
}

func TestContinuePlaytestKeepsSourceAndRecordedDice(t *testing.T) {
	cat, _ := game.LoadCatalog(content.FS)
	source := filepath.Join(t.TempDir(), "source")
	_, err := Run(context.Background(), cat, &fakeModel{combat: true}, Config{Directory: source, Scenario: "crossroads", Players: 20, Games: 1, Turns: 1, MaxCalls: 24})
	if err == nil {
		t.Fatal("fixture should stop at its budget")
	}
	original, err := os.ReadFile(filepath.Join(source, "campaigns.json"))
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "continued")
	s, err := Run(context.Background(), cat, &fakeModel{combat: true}, Config{From: source, Directory: dest, Games: 1, Turns: 2, MaxCalls: 100, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	if s.Players != 20 || s.Games[0].Turns != 2 || s.Games[0].Timings[0].PlayerSeconds > 0.01 {
		t.Fatalf("continuation resubmitted locked orders: %+v", s)
	}
	var beforeRooms, afterRooms []*game.Room
	if err := json.Unmarshal(original, &beforeRooms); err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(filepath.Join(dest, "campaigns.json"))
	if err := json.Unmarshal(copied, &afterRooms); err != nil {
		t.Fatal(err)
	}
	// Turn 1's outcomes retain the exact server roll text from the interrupted desk.
	for _, contest := range beforeRooms[0].Desk.Contests {
		if contest.Result == nil {
			continue
		}
		expected, _ := json.Marshal(contest.Result)
		// The source dice also remain in the first report's human-readable roll lines.
		found := false
		for _, entry := range afterRooms[0].Reports[0].Entries {
			for _, roll := range append(entry.Rolls, entry.Secret...) {
				if roll == contest.Title+": "+contest.Result.Text() {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("resumed turn lost a recorded contest: %s (%s)", contest.Title, expected)
		}
	}
	after, _ := os.ReadFile(filepath.Join(source, "campaigns.json"))
	if string(after) != string(original) {
		t.Fatal("source evidence changed")
	}
	calls, _ := os.ReadFile(filepath.Join(dest, "calls.jsonl"))
	if strings.Contains(string(calls), "turn 1 player") {
		t.Fatal("paid for players again during a locked turn")
	}
	dest2 := filepath.Join(t.TempDir(), "next")
	s, err = Run(context.Background(), cat, &fakeModel{combat: true}, Config{From: dest, Directory: dest2, Games: 1, Turns: 1, MaxCalls: 50, Concurrency: 4})
	if err != nil || s.Games[0].Turns != 1 || s.Games[0].Timings[0].Turn != 3 {
		t.Fatalf("next turn continuation: %+v %v", s, err)
	}
}
