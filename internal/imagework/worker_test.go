package imagework

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"asyncwargame/content"
	"asyncwargame/internal/game"
	"asyncwargame/internal/imageai"
)

func TestCancellationAndRecoveryNeverRetry(t *testing.T) {
	entered, released := make(chan struct{}), make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer api.Close()
	defer close(released)
	cat, err := game.LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := game.OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.Create("Images", "st-ines", "Ref", game.HashToken("ref"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := imageai.New(imageai.Config{BaseURL: api.URL, Model: "art"})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	queue := func(r *game.Room) error {
		err := r.QueueVignette(r.Referee(), 1, 0, "A squad crossing a bridge", "Crossing the bridge", client.Provider(), client.Model(), time.Now())
		if err == nil {
			id = r.BriefingVignette.ID
		}
		return err
	}
	if err := store.Update(code, queue); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: store, Client: client}
	done := make(chan struct{})
	go func() { worker.Once(context.Background()); close(done) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	if err := store.Update(code, func(r *game.Room) error { return r.DecideVignette(r.Referee(), 1, 0, id, "cancel") }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop worker")
	}
	store.Read(code, func(r *game.Room) {
		if r.BriefingVignette.Status != game.RunCancelled || r.BriefingVignette.File != "" {
			t.Error("late response changed cancelled draft")
		}
	})
	if err := store.Update(code, queue); err != nil {
		t.Fatal(err)
	}
	store.Update(code, func(r *game.Room) error { r.BriefingVignette.Status = game.RunRunning; return nil })
	reopened, err := game.OpenStore(path, cat)
	if err != nil {
		t.Fatal(err)
	}
	worker.Store = reopened
	worker.Recover()
	if worker.Once(context.Background()) {
		t.Fatal("restarted billed call retried automatically")
	}
	reopened.Read(code, func(r *game.Room) {
		if r.BriefingVignette.Status != game.RunFailed || r.BriefingVignette.Error == "" {
			t.Error("missing interrupted job explanation")
		}
	})
}
