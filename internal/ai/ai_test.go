package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteRetriesAndAuthenticates(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sekrit" {
			t.Errorf("request %s, auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-opencode-session") != "s-1" || r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("session %q, agent %q", r.Header.Get("x-opencode-session"), r.Header.Get("User-Agent"))
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct{ Role, Content string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "big-pickle" || len(req.Messages) != 2 || req.Messages[0].Role != "system" {
			t.Errorf("body %+v", req)
		}
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"<think>hmm</think>\nAll quiet."}}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL + "/v1/", APIKey: "sekrit", Model: "big-pickle", Name: "test", Session: "s-1"})
	c.backoff = time.Millisecond
	text, usage, err := c.Complete(context.Background(), "sys", "user", 100)
	if err != nil || text != "All quiet." || usage.Prompt != 12 || calls != 2 {
		t.Fatalf("text %q usage %+v err %v calls %d", text, usage, err, calls)
	}
}

func TestCompleteReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "OpenCode Zen"})
	if _, _, err := c.Complete(context.Background(), "s", "u", 10); err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("err %v", err)
	}
}

func TestDecodeJSON(t *testing.T) {
	var v struct{ Issues []struct{ Unit string } }
	reply := "Here you go:\n```json\n{\"issues\":[{\"unit\":\"m1\"}]}\n```\nHope that helps."
	if err := DecodeJSON(reply, &v); err != nil || len(v.Issues) != 1 || v.Issues[0].Unit != "m1" {
		t.Errorf("decoded %+v, %v", v, err)
	}
	if err := DecodeJSON("no json here", &v); err != ErrNoJSON {
		t.Errorf("err %v", err)
	}
}

func TestSessionIDsAreUUIDs(t *testing.T) {
	a, b := NewSessionID(), NewSessionID()
	if a == b || len(a) != 36 || a[14] != '4' || strings.Count(a, "-") != 4 {
		t.Errorf("session ids %q %q", a, b)
	}
}
