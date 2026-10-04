package playtest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"asyncwargame/internal/game"
)

// A synthetic load fixture, never installed in the user's scenario catalog.
// Commands are independent; every ten players share an enemy contact.
func loadCatalog(cat *game.Catalog, base string, players int) (*game.Catalog, string, error) {
	source := cat.Scenarios[base]
	if source == nil || players < 1 || players > 100 {
		return nil, "", fmt.Errorf("load fixture needs a scenario and 1–100 players")
	}
	sc := *source
	sc.ID = fmt.Sprintf("%s-load-%d", base, players)
	sc.Name = fmt.Sprintf("%s — %d-player load exercise", strings.TrimSuffix(source.Name, " (2 players)"), players)
	sc.Mission = nil
	sc.Formations = nil
	sc.Features = nil
	sc.Units = nil
	sc.Map = game.HexMap{Cols: 60, Rows: make([]string, 40)}
	for i := range sc.Map.Rows {
		sc.Map.Rows[i] = strings.Repeat(".", 60)
	}
	sc.Objectives = []string{"Coordinate your local detachment to screen the visible enemy. Keep survivors alive for the next turn."}
	sc.Briefing = []string{"Synthetic scale exercise. Each player commands one independent detachment. There are ten sectors, with up to ten friendly commands and one opposing unit each. Hold or maneuver locally, support nearby allies, and avoid unnecessary losses. Use the map and your private instructions."}
	sc.RefereeNotes = "This is a load exercise, not the original scenario. Each opposing unit probes its local sector. Apply only supported movement, observation and combat. No new units, construction or reinforcements. Never use the original scenario's place names or objectives."
	var friendly, enemy *game.ScenarioUnit
	mod := cat.Modules[sc.Module]
	for i := range source.Units {
		u := &source.Units[i]
		side := mod.Side(mod.Class(u.Class).Side)
		if side.Playable && friendly == nil {
			friendly = u
		}
		if side.Enemy && enemy == nil {
			enemy = u
		}
	}
	if friendly == nil || enemy == nil {
		return nil, "", fmt.Errorf("load fixture requires friendly and enemy templates")
	}
	for i := 0; i < players; i++ {
		sector, local := i/10, i%10
		u := *friendly
		u.ID = fmt.Sprintf("p%03d", i+1)
		u.Name = fmt.Sprintf("Detachment %03d", i+1)
		u.Formation = ""
		u.Known = true
		u.Spot = game.Spot{Col: 3 + (sector%5)*11 + local%5, Row: 3 + (sector/5)*18 + local/5}
		u.Brief = fmt.Sprintf("You command %s. Support nearby detachments without claiming authority over them.", u.Name)
		sc.Units = append(sc.Units, u)
	}
	for sector := 0; sector < (players+9)/10; sector++ {
		u := *enemy
		u.ID = fmt.Sprintf("e%03d", sector+1)
		u.Name = fmt.Sprintf("Opposition %02d", sector+1)
		u.Formation = ""
		u.Known = true
		u.Brief = ""
		u.Spot = game.Spot{Col: 8 + (sector%5)*11, Row: 4 + (sector/5)*18}
		sc.Units = append(sc.Units, u)
	}
	c := &game.Catalog{Modules: cat.Modules, Scenarios: make(map[string]*game.Scenario, len(cat.Scenarios)+1)}
	for id, v := range cat.Scenarios {
		c.Scenarios[id] = v
	}
	c.Scenarios[sc.ID] = &sc
	return c, sc.ID, nil
}

func playSeats(ctx context.Context, store *game.Store, rec *recorder, code string, seats []seat, turn, concurrency int, result *Result) error {
	// The commander gets a first pass so subordinate bots can see its shared order.
	formation := false
	store.Read(code, func(r *game.Room) { formation = len(r.Formations) > 0 })
	work := slices.Clone(seats)
	play := func(ctx context.Context, s seat, index int, local *Result) error {
		alive := false
		store.Read(code, func(r *game.Room) {
			for _, u := range r.Units {
				if u.AssignedTo == s.ID && !u.Destroyed() {
					alive = true
				}
			}
		})
		if !alive {
			return nil
		}
		label := fmt.Sprintf("%s turn %d player %d", code, turn, index+1)
		return playSeat(ctx, store, rec, code, s, local, label)
	}
	offset := 0
	if formation && len(work) > 0 {
		if e := play(ctx, work[0], 0, result); e != nil {
			return e
		}
		work = work[1:]
		offset = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for i := 0; i < min(max(1, concurrency), len(work)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				local := Result{}
				e := play(ctx, work[index], index+offset, &local)
				mu.Lock()
				result.Failures = append(result.Failures, local.Failures...)
				if e != nil && first == nil {
					first = e
					cancel()
				}
				mu.Unlock()
			}
		}()
	}
loop:
	for i := range work {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break loop
		}
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

type TurnTiming struct {
	Turn          int     `json:"turn"`
	PlayerSeconds float64 `json:"playerSeconds"`
	JudgeSeconds  float64 `json:"judgeSeconds"`
	AuditSeconds  float64 `json:"auditSeconds"`
}
type StageMetric struct {
	Stage          string  `json:"stage"`
	Calls          int     `json:"calls"`
	Failures       int     `json:"failures"`
	P50Seconds     float64 `json:"p50Seconds"`
	P95Seconds     float64 `json:"p95Seconds"`
	MaxPromptBytes int     `json:"maxPromptBytes"`
}
type callMetric struct {
	stage   string
	seconds float64
	prompt  int
	failed  bool
}

func (r *recorder) metrics() []StageMetric {
	r.mu.Lock()
	defer r.mu.Unlock()
	groups := map[string][]callMetric{}
	for _, v := range r.samples {
		groups[v.stage] = append(groups[v.stage], v)
	}
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out []StageMetric
	for _, k := range keys {
		list := groups[k]
		slices.SortFunc(list, func(a, b callMetric) int {
			if a.seconds < b.seconds {
				return -1
			}
			if a.seconds > b.seconds {
				return 1
			}
			return 0
		})
		m := StageMetric{Stage: k, Calls: len(list), P50Seconds: list[(len(list)-1)/2].seconds, P95Seconds: list[(len(list)*95-1)/100].seconds}
		for _, x := range list {
			m.MaxPromptBytes = max(m.MaxPromptBytes, x.prompt)
			if x.failed {
				m.Failures++
			}
		}
		out = append(out, m)
	}
	return out
}
func elapsed(start time.Time) float64 { return time.Since(start).Seconds() }
