// Package playtest runs real model-controlled seats against the normal AI referee.
// Every run gets a new, private store; it never opens the host's campaign data.
package playtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"asyncwargame/internal/ai"
	"asyncwargame/internal/assist"
	"asyncwargame/internal/game"
)

type Config struct {
	From                   string
	Players                int
	Concurrency            int
	CallTimeout            time.Duration
	Scenario               string
	Directory              string
	Games, Turns, MaxCalls int
	// Contact starts alternate games with a known enemy near the squad.
	Contact bool
}
type Summary struct {
	From            string        `json:"from,omitempty"`
	Players         int           `json:"players,omitempty"`
	Concurrency     int           `json:"concurrency"`
	Stages          []StageMetric `json:"stages,omitempty"`
	Provider        string        `json:"provider"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Model           string        `json:"model"`
	Started         time.Time     `json:"started"`
	Finished        time.Time     `json:"finished"`
	Calls           int           `json:"calls"`
	Tokens          int           `json:"tokens"`
	Games           []Result      `json:"games"`
}
type Result struct {
	Timings   []TurnTiming `json:"timings,omitempty"`
	Mission   string       `json:"mission,omitempty"`
	Code      string       `json:"code"`
	Fixture   string       `json:"fixture"`
	Turns     int          `json:"publishedTurns"`
	Destroyed []string     `json:"destroyed,omitempty"`
	Failures  []string     `json:"validationFailures,omitempty"`
	Error     string       `json:"error,omitempty"`
}
type seat struct{ ID, Profile string }
type recorder struct {
	mu      sync.Mutex
	samples []callMetric
	timeout time.Duration
	assist.Client
	file               *os.File
	max, calls, tokens int
	label              string
	err                error
}

func (r *recorder) Complete(ctx context.Context, system, user string, maxTokens int) (string, ai.Usage, error) {
	return r.complete(ctx, r.label, system, user, maxTokens)
}
func (r *recorder) complete(ctx context.Context, label, system, user string, maxTokens int) (string, ai.Usage, error) {
	r.mu.Lock()
	if r.err != nil {
		err := r.err
		r.mu.Unlock()
		return "", ai.Usage{}, err
	}
	if r.calls >= r.max {
		r.mu.Unlock()
		return "", ai.Usage{}, errors.New("playtest model call budget exhausted")
	}
	if err := ctx.Err(); err != nil {
		r.mu.Unlock()
		return "", ai.Usage{}, err
	}
	r.calls++
	call := r.calls
	log.Printf("playtest call %d/%d: %s", call, r.max, label)
	r.mu.Unlock()
	start := time.Now()
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	reply, usage, err := r.Client.Complete(ctx, system, user, maxTokens)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens += usage.Prompt + usage.Completion
	parts := strings.Fields(label)
	stage := "unknown"
	if len(parts) > 3 {
		stage = parts[3]
	}
	r.samples = append(r.samples, callMetric{stage: stage, seconds: elapsed(start), prompt: len(system) + len(user), failed: err != nil})
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	if r.err != nil {
		return "", usage, r.err
	}
	r.err = json.NewEncoder(r.file).Encode(map[string]any{"call": call, "stage": label, "seconds": time.Since(start).Seconds(), "system": system, "user": user, "reply": reply, "usage": usage, "error": detail})
	if r.err == nil {
		r.err = r.file.Sync()
	}
	if r.err != nil {
		return "", usage, fmt.Errorf("record model response: %w", r.err)
	}
	return reply, usage, err
}

// Run requires a directory that does not exist, and checkpoints results after
// each game. Calls are logical completions; the provider client may retry a
// transient HTTP failure up to three times within a completion.
func Run(ctx context.Context, cat *game.Catalog, client assist.Client, cfg Config) (summary Summary, err error) {
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 1
	}
	if cfg.Concurrency < 1 || cfg.Concurrency > 16 || cfg.Players < 0 || cfg.Players > 100 {
		return summary, errors.New("playtest supports 1–16 concurrent players and up to 100 seats")
	}
	if cfg.Scenario == "" {
		cfg.Scenario = "st-ines"
	}
	var previous []byte
	if cfg.From != "" {
		if cfg.Games != 1 || cfg.Players != 0 {
			return summary, errors.New("continue uses one existing game; do not specify playtest-players")
		}
		var e error
		cat, previous, cfg.Scenario, cfg.Players, e = loadPrevious(cat, cfg.From)
		if e != nil {
			return summary, e
		}
	} else if cfg.Players > 0 {
		var e error
		cat, cfg.Scenario, e = loadCatalog(cat, cfg.Scenario, cfg.Players)
		if e != nil {
			return summary, e
		}
	}
	if cat.Scenarios[cfg.Scenario] == nil {
		return summary, errors.New("unknown playtest scenario")
	}
	if cfg.Games < 1 || cfg.Games > 50 || cfg.Turns < 1 || cfg.Turns > 50 || cfg.MaxCalls < 1 || cfg.MaxCalls > 2000 || cfg.Directory == "" || client == nil {
		return summary, errors.New("playtest needs a new output directory, a model, 1–50 games/turns and 1–2000 calls")
	}
	if err = os.MkdirAll(filepath.Dir(cfg.Directory), 0700); err != nil {
		return
	}
	if err = os.Mkdir(cfg.Directory, 0700); err != nil {
		return summary, fmt.Errorf("playtest directory must be new: %w", err)
	}
	if cfg.Players > 0 || cfg.From != "" {
		path := filepath.Join(cfg.Directory, "scenarios")
		if err = os.Mkdir(path, 0700); err != nil {
			return
		}
		var b []byte
		b, err = json.MarshalIndent(cat.Scenarios[cfg.Scenario], "", "  ")
		if err != nil {
			return
		}
		if err = os.WriteFile(filepath.Join(path, cfg.Scenario+".json"), b, 0600); err != nil {
			return
		}
	}
	if previous != nil {
		if err = os.WriteFile(filepath.Join(cfg.Directory, "campaigns.json"), previous, 0600); err != nil {
			return
		}
	}
	f, err := os.OpenFile(filepath.Join(cfg.Directory, "calls.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return summary, err
	}
	defer f.Close()
	rec := &recorder{Client: client, file: f, max: cfg.MaxCalls, timeout: cfg.CallTimeout}
	summary = Summary{From: cfg.From, Players: cfg.Players, Concurrency: cfg.Concurrency, Provider: client.Provider(), Model: client.Model(), Started: time.Now().UTC()}
	if c, ok := client.(interface{ ReasoningEffort() string }); ok {
		summary.ReasoningEffort = c.ReasoningEffort()
	}
	defer func() {
		summary.Finished = time.Now().UTC()
		summary.Calls = rec.calls
		summary.Tokens = rec.tokens
		summary.Stages = rec.metrics()
		if e := saveSummary(cfg.Directory, summary); e != nil {
			err = errors.Join(err, e)
		}
	}()
	store, err := game.OpenStore(filepath.Join(cfg.Directory, "campaigns.json"), cat)
	if err != nil {
		return summary, err
	}
	for i := 0; i < cfg.Games; i++ {
		if ctx.Err() != nil || rec.calls >= cfg.MaxCalls {
			err = errors.Join(err, errors.New("playtest stopped at time/call limit"))
			break
		}
		result := Result{Fixture: cat.Scenarios[cfg.Scenario].Name}
		contact := cfg.Contact && cfg.Scenario == "st-ines" && i%2 == 1
		if contact {
			result.Fixture = "St. Ines, known shamblers one cell from squad (combat stress fixture)"
		}
		var code string
		var seats []seat
		var e error
		if cfg.From != "" {
			code, seats, e = previousSeats(store)
		} else {
			code, seats, e = setup(store, cfg.Scenario, contact)
		}
		result.Code = code
		if e == nil {
			e = runGame(ctx, store, rec, code, seats, cfg.Turns, cfg.Concurrency, &result)
		}
		if e != nil {
			result.Error = e.Error()
			err = errors.Join(err, fmt.Errorf("room %s: %w", code, e))
		}
		if code != "" {
			store.Read(code, func(r *game.Room) {
				if r.Mission != nil {
					result.Mission = r.Mission.Status
				}
				for _, u := range r.Units {
					if u.Destroyed() {
						result.Destroyed = append(result.Destroyed, u.ID)
					}
				}
			})
		}
		summary.Games = append(summary.Games, result)
		summary.Calls, summary.Tokens = rec.calls, rec.tokens
		if e := saveSummary(cfg.Directory, summary); e != nil {
			return summary, errors.Join(err, e)
		}
	}
	return summary, err
}
func saveSummary(dir string, s Summary) error {
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	p := filepath.Join(dir, "summary.json")
	if e = os.WriteFile(p+".tmp", append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}

var playerProfiles = []string{"Coordinate the squad toward the mission objectives. Prefer a global squad order unless separated.", "Aggressive: engage visible threats and protect the squad.", "Scout: seek a useful flanking position; break off when justified.", "Cautious: cover retreats, protect survivors, and conserve strength."}

func setup(store *game.Store, scenario string, contact bool) (string, []seat, error) {
	code, err := store.Create("AI playtest", scenario, "AI referee", newHash())
	if err != nil {
		return "", nil, err
	}
	var seats []seat
	err = store.Update(code, func(r *game.Room) error {
		ref, mod := r.Referee(), store.Cat.Modules[r.ModuleID]
		var playable []*game.Unit
		for _, u := range r.Units {
			if mod.Side(u.Side).Playable {
				playable = append(playable, u)
			}
		}
		if len(playable) < 1 || len(playable) > 100 {
			return errors.New("playtest supports 1–100 playable units")
		}
		for i, unit := range playable {
			profile := playerProfiles[i%len(playerProfiles)]
			if p := r.BotProfile(); p != "" {
				profile = p
			}
			m, e := r.RequestJoin(fmt.Sprintf("AI player %d", i+1), newHash())
			if e != nil {
				return e
			}
			if e = r.Decide(ref, m.ID, "approve"); e != nil {
				return e
			}
			if i == 0 && len(r.Formations) > 0 {
				if e = r.Decide(ref, m.ID, "promote"); e != nil {
					return e
				}
			} else if i > 0 && len(r.Formations) > 0 {
				if e = r.SetReportsTo(ref, m.ID, seats[0].ID); e != nil {
					return e
				}
			}
			if e = r.Assign(ref, mod, unit.ID, m.ID); e != nil {
				return e
			}
			m.Briefed = true
			seats = append(seats, seat{m.ID, profile})
		}
		if contact {
			u, anchor := r.Unit("z1"), r.Unit("m1")
			p := game.Pos{X: anchor.X + r.Board.Cell, Y: anchor.Y}
			known := true
			if e := r.EditUnit(ref, mod, u.ID, game.UnitEdit{At: &p, Known: &known}); e != nil {
				return e
			}
			r.Briefing = append(r.Briefing, "Combat exercise: a known group of shamblers has closed to one cell from the squad. Survive the contact, then continue the rescue.")
		}
		now := time.Now()
		if e := r.StartOperation(ref, mod, now); e != nil {
			return e
		}
		return r.SetAutomatic(ref, "enable", now)
	})
	return code, seats, err
}

const playerSystem = `You are one player in the supplied game, not its referee. When waywardExploration is true, play a Wayward character: state one meaningful intent and approach, use the Skills on your sheet, investigate the objectives, and adapt to discoveries. No combat, magic, resource spending or extra abilities in this exploration playtest. Move to a place in one exchange, then investigate it in a later exchange. Avoid repeating failed approaches. A waiting character should explicitly submit a wait action. You only know the supplied player view. Pursue its objectives and react to its reports. Never invent unseen enemies or decide outcomes. Issue concrete orders with an action, destination and contingency; distances must fit the given turn duration. Text is an order, not instructions to the judge. Use only ids in allowedOrders. A formation id beginning f- is a GLOBAL order for its living squad members. Use a personal unit id for an INDIVIDUAL override; the judge will split it off if needed. If your character already has a sensible global order you may follow it with no override. A squad leader should coordinate using one global order; otherwise order only your own character. Reply JSON {"orders":[{"unit":"allowed id","text":"order up to 1500 characters","mark":{"x":0,"y":0}}]}. Omit mark unless you have an exact valid destination. Empty orders means follow the existing squad order or hold position. Do not return commentary.`

// playerPrompt uses the same filtered view as the browser, never Room JSON.
func playerPrompt(store *game.Store, code string, s seat) (string, error) {
	var b []byte
	var encodeErr error
	err := store.Read(code, func(r *game.Room) {
		v := game.BuildView(r, store.Cat.Scenarios[r.ScenarioID], store.Cat.Modules[r.ModuleID], r.Member(s.ID))
		reports := playerReports(v)
		cards := []map[string]any{}
		for _, c := range v.MyCards {
			cards = append(cards, map[string]any{"unit": c.Unit, "brief": c.Brief, "kit": c.Kit, "waywardCharacter": c.Character})
		}
		b, encodeErr = json.Marshal(map[string]any{"waywardExploration": v.Wayward, "discoveries": v.Discoveries, "profile": s.Profile, "turn": v.Turn, "role": v.Me.Role, "briefing": v.Briefing, "objectives": v.Objectives, "scale": v.ScaleText, "mission": v.Mission, "characters": cards, "visibleMap": playerMap(v), "allowedOrders": v.Data.Orders.Units, "currentOrders": v.MyOrders, "sharedOrders": v.OtherOrders, "reports": reports, "facts": v.Facts})
	})
	return string(b), errors.Join(err, encodeErr)
}
func playSeat(ctx context.Context, store *game.Store, rec *recorder, code string, s seat, result *Result, label string) error {
	prompt, err := playerPrompt(store, code, s)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		reply, _, e := rec.complete(ctx, label, playerSystem, prompt, 1600)
		if e != nil {
			return e
		}
		var out struct {
			Orders *[]struct {
				Unit, Text string
				Mark       *game.Pos
			}
		}
		e = ai.DecodeJSON(reply, &out)
		if e == nil && (out.Orders == nil || len(*out.Orders) > 4) {
			e = errors.New("expected orders array with at most four orders")
		}
		if e == nil {
			e = store.Update(code, func(r *game.Room) error {
				actor, mod := r.Member(s.ID), store.Cat.Modules[r.ModuleID]
				v := game.BuildView(r, store.Cat.Scenarios[r.ScenarioID], mod, actor)
				allowed := map[string]bool{}
				for _, u := range v.Data.Orders.Units {
					allowed[u.ID] = true
				}
				used := map[string]bool{}
				for _, o := range *out.Orders {
					if !allowed[o.Unit] || used[o.Unit] {
						return errors.New("order target is unauthorized or repeated")
					}
					used[o.Unit] = true
					if _, e := r.SaveOrder(actor, mod, o.Unit, o.Text, o.Mark, true, time.Now()); e != nil {
						return e
					}
				}
				return nil
			})
		}
		if e == nil {
			return nil
		}
		result.Failures = append(result.Failures, label+": "+e.Error())
		prompt += "\nYour previous reply was rejected: " + e.Error() + ". Return corrected JSON."
	}
	return errors.New("player failed validation twice")
}
func runGame(ctx context.Context, store *game.Store, rec *recorder, code string, seats []seat, turns, concurrency int, result *Result) error {
	worker := &assist.Worker{Store: store, Client: rec}
	for round := 1; round <= turns; round++ {
		turn := 0
		phase := game.PhaseSetup
		living := 0
		complete := false
		store.Read(code, func(r *game.Room) {
			complete = r.MissionComplete()
			turn, phase = r.Turn, r.Phase
			for _, u := range r.Units {
				if store.Cat.Modules[r.ModuleID].Side(u.Side).Playable && !u.Destroyed() {
					living++
				}
			}
		})
		if complete || living == 0 {
			return nil
		}
		timing := TurnTiming{Turn: turn}
		started := time.Now()
		if phase == game.PhasePlanning {
			if err := playSeats(ctx, store, rec, code, seats, turn, concurrency, result); err != nil {
				return err
			}
		}
		timing.PlayerSeconds = elapsed(started)
		started = time.Now()
		before := map[string]game.Unit{}
		err := store.Update(code, func(r *game.Room) error {
			units := r.Units
			if r.Phase == game.PhaseResolving {
				units = r.LockedUnits
			}
			for _, u := range units {
				before[u.ID] = *u
			}
			if r.Phase == game.PhasePlanning {
				if !r.Assistant.Automatic {
					if err := r.SetAutomatic(r.Referee(), "enable", time.Now()); err != nil {
						return err
					}
				}
				return r.LockNow(r.Referee(), store.Cat.Modules[r.ModuleID], time.Now())
			}
			r.ResetRunning()
			if !r.HasPendingRun() {
				action := "retry"
				if !r.Assistant.Automatic {
					action = "enable"
				}
				return r.SetAutomatic(r.Referee(), action, time.Now())
			}
			return nil
		})
		if err != nil {
			return err
		}
		retries := map[string]int{}
		for steps := 0; steps < 200; steps++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			stage, target := "", ""
			store.Read(code, func(r *game.Room) {
				for _, x := range r.Assistant.Runs {
					if x.Status == game.RunPending {
						stage, target = x.Kind, x.Target
						break
					}
				}
			})
			rec.label = fmt.Sprintf("%s turn %d %s %s", code, turn, stage, target)
			if !worker.Once(ctx) {
				return errors.New("judge stalled without queued work")
			}
			published, failed, kind := false, "", ""
			store.Read(code, func(r *game.Room) {
				published = r.Phase == game.PhasePlanning && r.Turn == turn+1 || r.Phase == game.PhaseComplete && r.Turn == turn
				for _, x := range r.Assistant.Runs {
					if x.Status == game.RunFailed && x.Turn == turn {
						failed, kind = x.Error, x.Kind+"/"+x.Target
					}
				}
			})
			if published {
				timing.JudgeSeconds = elapsed(started)
				started = time.Now()
				if err := audit(store, code, turn, before); err != nil {
					return err
				}
				timing.AuditSeconds = elapsed(started)
				result.Timings = append(result.Timings, timing)
				result.Turns++
				log.Printf("playtest %s: published turn %d", code, turn)
				break
			}
			if failed != "" {
				result.Failures = append(result.Failures, rec.label+": "+failed)
				if retries[kind] >= 1 {
					return fmt.Errorf("judge %s failed twice: %s", kind, failed)
				}
				retries[kind]++
				if err := store.Update(code, func(r *game.Room) error { return r.SetAutomatic(r.Referee(), "retry", time.Now()) }); err != nil {
					return err
				}
			}
		}
		if result.Turns != round {
			return errors.New("judge exceeded stage limit")
		}
	}
	return nil
}
func audit(store *game.Store, code string, turn int, before map[string]game.Unit) error {
	var failures []string
	err := store.Read(code, func(r *game.Room) {
		mod := store.Cat.Modules[r.ModuleID]
		if len(r.Reports) != turn || len(r.PlansFor(r.Referee())) != 0 || r.Assistant.Decision != nil {
			failures = append(failures, "publication left an incomplete report or staged work")
		}
		for _, u := range r.Units {
			old, ok := before[u.ID]
			if !ok {
				failures = append(failures, "judge created an unrequested unit")
				continue
			}
			if u.Strength > old.Strength || u.Strength < 0 {
				failures = append(failures, "invalid strength for "+u.ID)
			}
			if old.Destroyed() && old.Pos() != u.Pos() {
				failures = append(failures, "destroyed unit moved: "+u.ID)
			}
			if reach, ok := r.ReachOf(mod, &old); ok && r.Cells(old.Pos(), u.Pos()) > reach.RunCells+0.01 {
				failures = append(failures, "excessive movement: "+u.ID)
			}
			if u.Destroyed() && r.CanOrder(mod, r.Referee(), u) {
				failures = append(failures, "destroyed unit accepts orders: "+u.ID)
			}
		}
		for _, m := range r.Members {
			if m.InPlay() {
				v := game.BuildView(r, store.Cat.Scenarios[r.ScenarioID], mod, m)
				if v.RefereeNotes != "" || v.Desk != nil || len(v.Intentions) > 0 {
					failures = append(failures, "referee-only data reached player view")
				}
				for _, rp := range v.Reports {
					for _, e := range rp.Entries {
						if len(e.Secret) > 0 || e.Seen == game.SeenByReferee {
							failures = append(failures, "private adjudication reached player report")
						}
					}
				}
			}
		}
	})
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func newHash() string { _, hash := game.NewToken(); return hash }

func (r *recorder) ReasoningEffort() string {
	if c, ok := r.Client.(interface{ ReasoningEffort() string }); ok {
		return c.ReasoningEffort()
	}
	return ""
}
