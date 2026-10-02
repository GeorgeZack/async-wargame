// Package ai talks to a language model over the OpenAI-compatible chat
// completions API. That one shape covers hosted gateways such as OpenCode
// Zen and local servers such as Ollama, llama.cpp's llama-server, and LM
// Studio. The rest of the server only sees Client.Complete.
package ai

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Presets for -ai. Anything else is taken as a base URL.
var Presets = map[string]Preset{
	// OpenCode Go: the subscription endpoint.
	"opencode": {BaseURL: "https://opencode.ai/zen/go/v1", Model: "glm-5.2", Name: "OpenCode", NeedsKey: true, Session: true},
	// OpenCode Zen: pay as you go, with some free models.
	"opencode-zen": {BaseURL: "https://opencode.ai/zen/v1", Model: "big-pickle", Name: "OpenCode Zen", NeedsKey: true, Session: true},
	"ollama":       {BaseURL: "http://127.0.0.1:11434/v1", Model: "llama3.1", Name: "Ollama on this machine"},
}

// Preset is a known provider.
type Preset struct {
	BaseURL, Model, Name string
	NeedsKey             bool
	Session              bool // send an x-opencode-session header
}

// UserAgent identifies the server to model providers.
const UserAgent = "async-wargame/1.0"

// Config says which model to use.
type Config struct {
	BaseURL string // ending in /v1
	APIKey  string // sent as a bearer token if set
	Model   string
	Name    string // shown to referees: where game data goes
	Timeout time.Duration
	// Session, if set, is sent as x-opencode-session: OpenCode groups a
	// client's requests by it. One per server run.
	Session string
}

// NewSessionID makes a random UUID (version 4) for Config.Session.
func NewSessionID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// header sets what every request carries.
func (c *Client) header(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	if c.cfg.Session != "" {
		req.Header.Set("x-opencode-session", c.cfg.Session)
	}
}

// Client calls the model.
type Client struct {
	cfg     Config
	http    *http.Client
	backoff time.Duration // base wait between retries
}

// New makes a client.
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Minute
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}, backoff: 5 * time.Second}
}

// Model is the model's ID; Provider is where requests go, for referees.
func (c *Client) Model() string    { return c.cfg.Model }
func (c *Client) Provider() string { return c.cfg.Name }

// Usage is what one call cost, in tokens.
type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Complete sends a system prompt and a user message and returns the reply.
// It retries a couple of times when the service is busy or briefly down.
func (c *Client) Complete(ctx context.Context, system, user string, maxTokens int) (string, Usage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       c.cfg.Model,
		"messages":    []message{{"system", system}, {"user", user}},
		"temperature": 0.3,
		"max_tokens":  maxTokens,
		"stream":      false,
	})
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", Usage{}, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * c.backoff):
			}
		}
		text, usage, retry, err := c.once(ctx, body)
		if err == nil {
			return text, usage, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return "", Usage{}, lastErr
}

func (c *Client) once(ctx context.Context, body []byte) (string, Usage, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.header(req)
	res, err := c.http.Do(req)
	if err != nil {
		return "", Usage{}, ctx.Err() == nil, fmt.Errorf("%s: %w", c.cfg.Name, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		retry := res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500
		return "", Usage{}, retry, fmt.Errorf("%s: %s: %s", c.cfg.Name, res.Status, errorText(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", Usage{}, false, fmt.Errorf("%s: unreadable reply: %w", c.cfg.Name, err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", out.Usage, true, fmt.Errorf("%s: empty reply", c.cfg.Name)
	}
	return stripThinking(out.Choices[0].Message.Content), out.Usage, false, nil
}

// errorText pulls the message out of an OpenAI-style error body.
func errorText(raw []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// stripThinking drops a <think>…</think> preamble some local models emit.
func stripThinking(s string) string {
	if i := strings.Index(s, "</think>"); i >= 0 && strings.HasPrefix(strings.TrimSpace(s), "<think>") {
		s = s[i+len("</think>"):]
	}
	return strings.TrimSpace(s)
}

// ErrNoJSON means the reply had no JSON object in it.
var ErrNoJSON = errors.New("the model's reply had no JSON object in it")

// DecodeJSON finds the JSON object in a reply (models sometimes wrap it in
// prose or a code fence) and decodes it into v.
func DecodeJSON(reply string, v any) error {
	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return ErrNoJSON
	}
	return json.Unmarshal([]byte(reply[start:end+1]), v)
}

// Models lists the model IDs the service offers, to check the configured
// one exists. Not every server implements it; callers treat errors as
// "can't tell".
func (c *Client) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.header(req)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, len(out.Data))
	for i, m := range out.Data {
		ids[i] = m.ID
	}
	return ids, nil
}
