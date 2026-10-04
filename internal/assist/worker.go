// Package assist runs the referee assistant's model work in the background:
// it takes queued runs from rooms, asks the model, and hands the replies
// back. Game state is only touched through the store, and no lock is held
// while the model thinks.
package assist

import (
	"context"
	"errors"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

// Worker bounds model requests across rooms. Each room claims one request at a
// time; the default concurrency of one also suits local models.
type Worker struct {
	Retries     int
	Concurrency int
	cursor      atomic.Uint64
	Store       *game.Store
	Client      Client
	// Every is how often to look for queued work.
	Every time.Duration
}

// Client also permits a playtest recorder to enforce a shared call budget.
type Client interface {
	Complete(context.Context, string, string, int) (string, ai.Usage, error)
	Model() string
	Provider() string
}

// Run works until ctx is done.
func (w *Worker) Run(ctx context.Context) {
	if w.Every == 0 {
		w.Every = 5 * time.Second
	}
	// Runs a restart interrupted go back in the queue.
	for _, code := range w.Store.Codes() {
		w.Store.Touch(code, func(r *game.Room) bool { return r.ResetRunning() })
	}
	var wg sync.WaitGroup
	for i := 0; i < min(max(w.Concurrency, 1), 16); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(w.Every)
			defer t.Stop()
			for {
				if ctx.Err() != nil {
					return
				}
				if w.Once(ctx) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}
	wg.Wait()
}

// Once does one queued run, if there is one, and reports whether it did.
func (w *Worker) Once(ctx context.Context) bool {
	codes := w.Store.Codes()
	slices.Sort(codes)
	if len(codes) == 0 {
		return false
	}
	start := int(w.cursor.Add(1)-1) % len(codes)
	for i := range codes {
		code := codes[(start+i)%len(codes)]
		pending := false
		w.Store.Read(code, func(r *game.Room) { pending = r.HasPendingRun() })
		if !pending {
			continue
		}
		var run *game.Run
		var system, user string
		var ok bool
		var localReply string
		var local bool
		err := w.Store.UpdateAssistant(code, func(r *game.Room) error {
			run = r.NextRun(w.Client.Model(), w.Client.Provider(), time.Now())
			if run == nil {
				return errNothing
			}
			if c, ok := w.Client.(interface{ ReasoningEffort() string }); ok {
				run.ReasoningEffort = c.ReasoningEffort()
				for _, stored := range r.Assistant.Runs {
					if stored.ID == run.ID {
						stored.ReasoningEffort = run.ReasoningEffort
					}
				}
			}
			system, user, ok = r.PromptFor(w.Store.Cat.Modules[r.ModuleID], run)
			if ok {
				localReply, local = r.AutomaticReply(w.Store.Cat.Modules[r.ModuleID], run)
			}
			return nil
		})
		if err != nil || run == nil {
			continue
		}
		var reply string
		var usage ai.Usage
		var callErr error
		if len(system)+len(user) > 256000 {
			callErr = errors.New("adjudication prompt exceeds 256 KB; simplify the scenario context before retrying")
		} else if local {
			reply = localReply
		} else if ok {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			stop := w.watchCancel(callCtx, cancel, code, run.ID)
			maxTokens := map[string]int{game.RunSuggest: 2500, game.RunOutcome: 600, game.RunNarrative: 600,
				game.RunIntentions: 1500, game.RunQuestion: 300, game.RunAutoIntentions: 2000, game.RunAutoContests: 5000, game.RunAutoResolve: 10000, game.RunAutoPublish: 2000}[run.Kind]
			reply, usage, callErr = w.Client.Complete(callCtx, system, user, maxTokens)
			stop()
			cancel()
		}
		err = w.Store.UpdateAssistant(code, func(r *game.Room) error {
			if ctx.Err() != nil {
				for _, stored := range r.Assistant.Runs {
					if stored.ID == run.ID && stored.Status == game.RunRunning {
						stored.Tokens += usage.Prompt + usage.Completion
					}
				}
				r.ResetRunning()
				return nil
			}
			r.FinishRun(run.ID, usage.Prompt+usage.Completion, callErr, func(stored *game.Run) error {
				if !ok {
					if stored.Automatic() {
						return errors.New("automatic turn is no longer available; pause or retry")
					}
					return nil // nothing to do: done, quietly
				}
				return r.ApplyReply(w.Store.Cat.Modules[r.ModuleID], stored, reply, ai.DecodeJSON)
			}, !ok, time.Now())
			for _, stored := range r.Assistant.Runs {
				if stored.ID == run.ID && stored.Status == game.RunFailed && stored.Automatic() && reply != "" && len(reply) <= 64000 {
					stored.RejectedReply = reply
				}
			}
			r.RetryAutomatic(run.ID, w.Retries, time.Now())
			return nil
		})
		if err != nil {
			log.Printf("assistant, room %s: %v", code, err)
		}
		return true
	}
	return false
}

// watchCancel cuts the model call short if the referee cancels the run
// (or switches the assistant off) while it's waiting. It returns a
// function that stops watching.
func (w *Worker) watchCancel(ctx context.Context, cancel context.CancelFunc, code, runID string) func() {
	changes, unsubscribe := w.Store.Subscribe(code)
	done := make(chan struct{})
	go func() {
		defer unsubscribe()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-changes:
				stopped := false
				w.Store.Read(code, func(r *game.Room) { stopped = r.RunCancelledNow(runID) })
				if stopped {
					cancel()
					return
				}
			}
		}
	}()
	return func() { close(done) }
}

type nothing struct{}

func (nothing) Error() string { return "nothing to do" }

var errNothing error = nothing{}
