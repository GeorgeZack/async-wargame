package ai

import (
	"context"
	"encoding/json"
	"io"
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

func TestInstructionsJoinEveryPrompt(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		got = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "t", Instructions: "Answer directly.\nNo long reasoning."})
	if _, _, err := c.Complete(context.Background(), "You are Jev.", "hi", 50); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "You are Jev.") || !strings.HasSuffix(got, "Answer directly.\nNo long reasoning.") {
		t.Errorf("system prompt %q", got)
	}
	c = New(Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "t"})
	c.Complete(context.Background(), "You are Jev.", "hi", 50)
	if got != "You are Jev." {
		t.Errorf("without instructions: %q", got)
	}
}

func TestTruncatedRepliesNeverPublishAndKeepUsage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"The squad moved to the"},"finish_reason":"length"}],"usage":{"prompt_tokens":100,"completion_tokens":10}}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Model: "test", Name: "test"})
	text, usage, err := c.Complete(context.Background(), "s", "u", 10)
	if err == nil || text != "" || usage.Prompt != 100 || usage.Completion != 10 || calls != 1 {
		t.Fatalf("truncation not rejected: %q %+v %v %d", text, usage, err, calls)
	}
}
func TestRetryUsageIsAccumulated(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		reply := ""
		if calls == 2 {
			reply = "Done."
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}, "usage": Usage{Prompt: 10, Completion: 5}})
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Model: "test", Name: "test"})
	c.backoff = time.Millisecond
	_, usage, err := c.Complete(context.Background(), "s", "u", 10)
	if err != nil || calls != 2 || usage.Prompt != 20 || usage.Completion != 10 {
		t.Fatalf("lost failed attempt's usage: %+v %v", usage, err)
	}
}

func TestOptionalReasoningEffort(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	for _, effort := range []string{"", "low"} {
		got = map[string]any{}
		c := New(Config{BaseURL: srv.URL, Model: "test", ReasoningEffort: effort})
		if _, _, err := c.Complete(context.Background(), "s", "u", 100); err != nil {
			t.Fatal(err)
		}
		value, has := got["reasoning_effort"]
		if effort == "" && has || effort != "" && value != effort {
			t.Fatalf("effort %q sent %v", effort, got)
		}
	}
}

type captureTransport func(*http.Request) (*http.Response, error)

func (f captureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenRouterReasoningWireFormat(t *testing.T) {
	for _, effort := range []string{"", "low", "none"} {
		c := New(Config{BaseURL: "https://openrouter.ai/api/v1", Model: "test", ReasoningEffort: effort})
		c.http.Transport = captureTransport(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, ok := body["reasoning_effort"]; ok {
				t.Fatal("OpenRouter needs the reasoning object")
			}
			reasoning, present := body["reasoning"].(map[string]any)
			if effort == "" && present || effort == "low" && reasoning["effort"] != "low" || effort == "none" && reasoning["enabled"] != false {
				t.Fatalf("effort %s: %v", effort, body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)), Header: make(http.Header)}, nil
		})
		if _, _, err := c.Complete(context.Background(), "s", "u", 100); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSONModeOnlyOnStructuredPrompts(t *testing.T) {
	var formats []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		formats = append(formats, req["response_format"])
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer srv.Close()
	on := New(Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "test", JSONMode: true})
	off := New(Config{BaseURL: srv.URL + "/v1", Model: "m", Name: "test"})
	on.Complete(context.Background(), `Decide. Reply JSON {"contests":[]}`, "u", 10)
	on.Complete(context.Background(), "Write a short situation briefing. Plain prose.", "u", 10)
	off.Complete(context.Background(), `Decide. Reply JSON {"contests":[]}`, "u", 10)
	if len(formats) != 3 || formats[0] == nil || formats[1] != nil || formats[2] != nil {
		t.Fatalf("response_format per call: %v", formats)
	}
}
