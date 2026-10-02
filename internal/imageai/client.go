// Package imageai calls an independently configured Images API. It never
// inherits the text assistant's endpoint, credentials, model, or instructions.
package imageai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxBytes = 15 << 20

type Config struct {
	BaseURL, Model, APIKey string
	Size                   string
	// Empty uses the provider default (GPT Image returns base64). Some
	// compatible gateways require an explicit "b64_json" instead.
	ResponseFormat string
	Timeout        time.Duration
}

type Client struct {
	cfg      Config
	http     *http.Client
	provider string
}

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("image-api must be an http(s) base URL without credentials, query, or fragment")
	}
	cfg.BaseURL = strings.TrimRight(u.String(), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Model == "" {
		return nil, fmt.Errorf("image-model is required when image-api is set")
	}
	if cfg.ResponseFormat != "" && cfg.ResponseFormat != "b64_json" {
		return nil, fmt.Errorf("image-response-format must be empty or b64_json")
	}
	if cfg.Size == "" {
		cfg.Size = "1024x1024"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &Client{cfg: cfg, provider: u.Host, http: &http.Client{Timeout: cfg.Timeout,
		// Never forward image credentials through a provider redirect.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Model() string    { return c.cfg.Model }
func (c *Client) Provider() string { return c.provider }

type Result struct {
	Bytes         []byte
	Extension     string
	Width, Height int
}

// Generate makes exactly one request. A failed/ambiguous call is never retried
// automatically: the provider may already have billed for an image.
func (c *Client) Generate(ctx context.Context, prompt string) (Result, error) {
	var zero Result
	body := map[string]any{"model": c.cfg.Model, "prompt": prompt, "n": 1, "size": c.cfg.Size}
	if c.cfg.ResponseFormat != "" {
		body["response_format"] = c.cfg.ResponseFormat
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/images/generations", bytes.NewReader(raw))
	if err != nil {
		return zero, fmt.Errorf("Could not prepare the image request.")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "async-wargame/1.0")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return zero, fmt.Errorf("Image generation was interrupted or timed out.")
		}
		return zero, fmt.Errorf("Could not reach the image service, or the request timed out.")
	}
	defer res.Body.Close()
	// Provider errors can echo prompts or credentials. Store only the status.
	if res.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("Image service returned HTTP %d. Check its credentials, model, and quota before trying again.", res.StatusCode)
	}
	const limit = MaxBytes*4/3 + (1 << 20)
	raw, err = io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(raw) > limit {
		return zero, fmt.Errorf("Image service returned an unreadable or oversized response.")
	}
	var reply struct {
		Data []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &reply) != nil || len(reply.Data) != 1 || reply.Data[0].Base64 == "" {
		return zero, fmt.Errorf("Image service must return one base64 image (b64_json); URL-only responses are not supported.")
	}
	b, err := base64.StdEncoding.DecodeString(reply.Data[0].Base64)
	if err != nil || len(b) > MaxBytes {
		return zero, fmt.Errorf("Image service returned invalid image data or more than 15 MB.")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 100 || cfg.Height < 100 || cfg.Width > 20000 || cfg.Height > 20000 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return zero, fmt.Errorf("Image service must return a PNG or JPEG between 100 and 20,000 pixels per side, up to 40 megapixels.")
	}
	// Decode too, so a valid header on a truncated file cannot become a preview.
	if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
		return zero, fmt.Errorf("Image service returned a damaged image.")
	}
	ext := ".png"
	if format == "jpeg" {
		ext = ".jpg"
	}
	return Result{Bytes: b, Extension: ext, Width: cfg.Width, Height: cfg.Height}, nil
}
