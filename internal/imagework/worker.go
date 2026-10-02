// Package imagework runs vignette jobs independently from the text assistant.
package imagework

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"asyncwargame/internal/game"
	"asyncwargame/internal/imageai"
)

type Worker struct {
	Store  *game.Store
	Client *imageai.Client
	Every  time.Duration
}

var errStale = errors.New("image job no longer active")

func (w *Worker) Recover() {
	for _, code := range w.Store.Codes() {
		busy := false
		w.Store.Read(code, func(r *game.Room) {
			for _, d := range r.Vignettes() {
				busy = busy || d.Status == game.RunRunning
			}
		})
		if !busy {
			continue
		}
		if err := w.Store.Update(code, func(r *game.Room) error {
			for _, d := range r.Vignettes() {
				if d.Status == game.RunRunning {
					d.Status, d.Error = game.RunFailed, "The server stopped during generation. No automatic retry was made; the service may already have charged for it."
				}
			}
			return nil
		}); err != nil {
			log.Printf("image recovery, room %s: %v", code, err)
		}
	}
}

func (w *Worker) Run(ctx context.Context) {
	w.Recover()
	every := w.Every
	if every <= 0 {
		every = time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		for ctx.Err() == nil && w.Once(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Once claims one queued job. Requests are serialized and never retried.
func (w *Worker) Once(ctx context.Context) bool {
	for _, code := range w.Store.Codes() {
		var job game.Vignette
		w.Store.Read(code, func(r *game.Room) {
			if r.Closed {
				return
			}
			for _, d := range r.Vignettes() {
				if d.Status == game.RunPending {
					job = *d
					break
				}
			}
		})
		if job.ID == "" {
			continue
		}
		err := w.Store.Update(code, func(r *game.Room) error {
			d := r.Vignette(job.Operation, job.Turn)
			if r.Closed || d == nil || d.ID != job.ID || d.Status != game.RunPending {
				return errStale
			}
			if d.Model != w.Client.Model() || d.Provider != w.Client.Provider() {
				d.Status, d.Error = game.RunFailed, "The image service settings changed. Review them and generate again."
				return nil
			}
			d.Status = game.RunRunning
			job = *d
			return nil
		})
		if err != nil {
			continue
		}
		if job.Status != game.RunRunning {
			return true
		}
		w.generate(ctx, code, job)
		return true
	}
	return false
}

func (w *Worker) generate(ctx context.Context, code string, job game.Vignette) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	changes, unsubscribe := w.Store.Subscribe(code)
	defer unsubscribe()
	done := make(chan struct{})
	defer close(done)
	active := func() bool {
		ok := false
		w.Store.Read(code, func(r *game.Room) {
			d := r.Vignette(job.Operation, job.Turn)
			ok = !r.Closed && d != nil && d.ID == job.ID && d.Status == game.RunRunning
		})
		return ok
	}
	if !active() {
		return
	}
	go func() {
		for {
			select {
			case <-done:
				return
			case <-callCtx.Done():
				return
			case <-changes:
				if !active() {
					cancel()
					return
				}
			}
		}
	}()
	result, callErr := w.Client.Generate(callCtx, game.VignettePrompt(job.Prompt))
	err := w.Store.Update(code, func(r *game.Room) error {
		d := r.Vignette(job.Operation, job.Turn)
		if d == nil || d.ID != job.ID || d.Status != game.RunRunning {
			return errStale
		}
		if r.Closed {
			d.Status, d.Error = game.RunCancelled, "Room closed during generation."
			return nil
		}
		if callErr != nil {
			d.Status, d.Error = game.RunFailed, callErr.Error()
			return nil
		}
		name := fmt.Sprintf("%s-vignette-%s%s", code, job.ID, result.Extension)
		if err := save(w.Store.MapDir(), name, result.Bytes); err != nil {
			log.Printf("saving vignette, room %s: %v", code, err)
			d.Status, d.Error = game.RunFailed, "The image was generated but could not be saved. Check server storage before trying again."
			return nil
		}
		d.Status, d.File, d.Width, d.Height = game.RunDone, name, result.Width, result.Height
		return nil
	})
	if err != nil && !errors.Is(err, errStale) {
		log.Printf("image completion, room %s: %v", code, err)
	}
}

func save(dir, name string, b []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".vignette-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}
