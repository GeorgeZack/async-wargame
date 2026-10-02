package imageai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateIndependentRequestAndValidation(t *testing.T) {
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 128, 128)))
	status, payload, calls := 200, base64.StdEncoding.EncodeToString(pic.Bytes()), 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/images/generations" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer image-secret" || r.Header.Get("x-opencode-session") != "" {
			t.Error("incorrect image request")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "separate-image-model" || body["prompt"] != "a scene" || body["response_format"] != nil || body["n"] != float64(1) {
			t.Errorf("bad body: %v", body)
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": payload}}})
	}))
	defer ts.Close()
	c, err := New(Config{BaseURL: ts.URL + "/v1", Model: "separate-image-model", APIKey: "image-secret"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Generate(context.Background(), "a scene")
	if err != nil || out.Width != 128 || !bytes.Equal(out.Bytes, pic.Bytes()) {
		t.Fatalf("generate: %+v, %v", out, err)
	}
	for _, bad := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("not an image")), base64.StdEncoding.EncodeToString(pic.Bytes()[:40])} {
		payload = bad
		if _, err := c.Generate(context.Background(), "a scene"); err == nil {
			t.Error("accepted invalid image")
		}
	}
	status, calls = 429, 0
	if _, err := c.Generate(context.Background(), "a scene"); err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("missing status error: %v", err)
	}
	if calls != 1 {
		t.Error("retried a potentially billed request")
	}
}

func TestConfigValidation(t *testing.T) {
	for _, cfg := range []Config{{BaseURL: "https://example.com/v1"}, {BaseURL: "file:///tmp/a", Model: "x"}, {BaseURL: "https://key@example.com/v1", Model: "x"}, {BaseURL: "https://example.com/v1?key=secret", Model: "x"}, {BaseURL: "https://example.com/v1", Model: "x", ResponseFormat: "url"}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("accepted invalid config: %+v", cfg)
		}
	}
}
