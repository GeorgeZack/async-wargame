// Package assist runs the referee assistant's model work in the background:
// it takes queued runs from rooms, asks the model, and hands the replies
// back. Game state is only touched through the store, and no lock is held
// while the model thinks.
package assist

import (
	"context"
	"log"
	"time"

	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

// Worker processes runs one at a time: a single model is shared by every
// room on the server, and local models in particular handle one request at
// a time best.
type Worker struct {
	Store  *game.Store
	Client *ai.Client
	// Every is how often to look for queued work.
	Every time.Duration
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
	t := time.NewTicker(w.Every)
	defer t.Stop()
	for {
		for w.Once(ctx) {
			if ctx.Err() != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Once does one queued run, if there is one, and reports whether it did.
func (w *Worker) Once(ctx context.Context) bool {
	for _, code := range w.Store.Codes() {
		pending := false
		w.Store.Read(code, func(r *game.Room) { pending = r.HasPendingRun() })
		if !pending {
			continue
		}
		var run *game.Run
		var system, user string
		var ok bool
		err := w.Store.Update(code, func(r *game.Room) error {
			run = r.NextRun(w.Client.Model(), w.Client.Provider(), time.Now())
			if run == nil {
				return errNothing
			}
			system, user, ok = r.PromptFor(w.Store.Cat.Modules[r.ModuleID], run)
			return nil
		})
		if err != nil || run == nil {
			continue
		}
		var reply string
		var usage ai.Usage
		var callErr error
		if ok {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			maxTokens := map[string]int{game.RunSuggest: 2500, game.RunOutcome: 600, game.RunNarrative: 600,
				game.RunIntentions: 1500, game.RunQuestion: 300}[run.Kind]
			reply, usage, callErr = w.Client.Complete(callCtx, system, user, maxTokens)
			cancel()
		}
		err = w.Store.Update(code, func(r *game.Room) error {
			r.FinishRun(run.ID, usage.Prompt+usage.Completion, callErr, func(stored *game.Run) error {
				if !ok {
					return nil // nothing to do: done, quietly
				}
				return r.ApplyReply(w.Store.Cat.Modules[r.ModuleID], stored, reply, ai.DecodeJSON)
			}, !ok, time.Now())
			return nil
		})
		if err != nil {
			log.Printf("assistant, room %s: %v", code, err)
		}
		return true
	}
	return false
}

type nothing struct{}

func (nothing) Error() string { return "nothing to do" }

var errNothing error = nothing{}
