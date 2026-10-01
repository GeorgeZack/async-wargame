package ai

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// When run with AW_FAKE_LLAMA=1, the test binary pretends to be llama-server:
// it takes the same flags, is "loading" for a moment, then answers chat.
func TestMain(m *testing.M) {
	if os.Getenv("AW_FAKE_LLAMA") == "1" {
		fakeLlama()
		return
	}
	os.Exit(m.Run())
}

func fakeLlama() {
	fs := flag.NewFlagSet("llama-server", flag.ExitOnError)
	model := fs.String("m", "", "")
	host := fs.String("host", "", "")
	port := fs.Int("port", 0, "")
	fs.Int("c", 0, "")
	alias := fs.String("alias", "", "")
	fs.Parse(os.Args[1:])
	ready := time.Now().Add(700 * time.Millisecond)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Before(ready) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"local %s (%s)"}}]}`, *alias, filepath.Base(*model))
	})
	http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), nil)
}

func TestStartLocal(t *testing.T) {
	model := filepath.Join(t.TempDir(), "tiny.gguf")
	os.WriteFile(model, []byte("not really a model"), 0o644)
	self, _ := os.Executable()
	t.Setenv("AW_FAKE_LLAMA", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	l, err := StartLocal(ctx, self, model, 4096, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := New(Config{BaseURL: l.BaseURL, Model: "tiny.gguf", Name: "local"})
	text, _, err := c.Complete(ctx, "s", "u", 10)
	if err != nil || text != "local tiny.gguf (tiny.gguf)" {
		t.Errorf("reply %q, %v", text, err)
	}
	l.Stop()
	select {
	case <-l.Exited():
	case <-time.After(5 * time.Second):
		t.Error("the local server didn't stop")
	}
	if _, err := StartLocal(ctx, self, filepath.Join(t.TempDir(), "missing.gguf"), 4096, nil); err == nil {
		t.Error("started without a model file")
	}
}
