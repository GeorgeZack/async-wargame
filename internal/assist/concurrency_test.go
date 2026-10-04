package assist

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"asyncwargame/content"
	"asyncwargame/internal/ai"
	"asyncwargame/internal/game"
)

type blockedClient struct {
	started      chan struct{}
	active, peak atomic.Int32
}

func (*blockedClient) Model() string    { return "blocked-test" }
func (*blockedClient) Provider() string { return "test" }
func (c *blockedClient) Complete(ctx context.Context, _, _ string, _ int) (string, ai.Usage, error) {
	n := c.active.Add(1)
	defer c.active.Add(-1)
	for old := c.peak.Load(); n > old; old = c.peak.Load() {
		if c.peak.CompareAndSwap(old, n) {
			break
		}
	}
	c.started <- struct{}{}
	<-ctx.Done()
	return "", ai.Usage{}, ctx.Err()
}
func TestWorkerConcurrencyAndCancellation(t *testing.T) {
	cat, e := game.LoadCatalog(content.FS)
	if e != nil {
		t.Fatal(e)
	}
	store, e := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"First", "Second", "Third"} {
		code, e := store.Create(name, "crossroads", "Ref", game.HashToken(name))
		if e != nil {
			t.Fatal(e)
		}
		e = store.Update(code, func(r *game.Room) error {
			if e := r.StartOperation(r.Referee(), cat.Modules[r.ModuleID], time.Now()); e != nil {
				return e
			}
			if e := r.SetAssistant(r.Referee(), true, false, false); e != nil {
				return e
			}
			return r.RunNow(r.Referee(), game.RunIntentions, "", time.Now())
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	client := &blockedClient{started: make(chan struct{}, 10)}
	w := &Worker{Store: store, Client: client, Concurrency: 2, Every: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-client.started:
		case <-time.After(5 * time.Second):
			t.Fatal("one slow room blocked other rooms")
		}
	}
	if client.peak.Load() != 2 {
		t.Fatalf("wrong request concurrency %d", client.peak.Load())
	}
	select {
	case <-client.started:
		t.Fatal("exceeded request concurrency")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not cancel")
	}
	if client.active.Load() != 0 {
		t.Fatal("model request leaked")
	}
}
