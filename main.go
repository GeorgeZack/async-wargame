// Command async-wargame runs a campaign server: one binary serving the
// browser client, room membership, and the campaign record.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // turn schedules use named time zones; don't depend on the host's zoneinfo

	"asyncwargame/content"
	"asyncwargame/internal/ai"
	"asyncwargame/internal/assist"
	"asyncwargame/internal/game"
	"asyncwargame/internal/imageai"
	"asyncwargame/internal/imagework"
	"asyncwargame/internal/playtest"
	"asyncwargame/internal/push"
	"asyncwargame/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address (use :8080 to accept LAN connections)")
	dataPath := flag.String("data", "data/campaigns.json", "campaign state file")
	modsDir := flag.String("modules", "", "extra directory with modules/*.json and scenarios/*.json to load alongside the built-in ones")
	aiFlag := flag.String("ai", "", `AI model for the referee assistant: "opencode" (OpenCode Go) or "opencode-zen" (key in AW_AI_KEY or OPENCODE_API_KEY), "ollama", "local" (runs llama-server with -ai-local-model), or an OpenAI-compatible base URL ending in /v1. Empty: no AI`)
	aiRetries := flag.Int("ai-retries", 1, "automatic retries per failed unchanged referee stage (0–2); dice are preserved")
	aiConcurrency := flag.Int("ai-concurrency", 1, "concurrent referee requests across rooms (1–16); each room stays sequential")
	aiEffort := flag.String("ai-reasoning-effort", "", "optional reasoning effort supported by your provider/model: none, minimal, low, medium, high, xhigh or max; empty keeps its default")
	aiModel := flag.String("ai-model", "", "model ID (default: glm-5.2 for opencode, big-pickle for opencode-zen, llama3.1 for ollama)")
	aiLlama := flag.String("ai-llama-server", "llama-server", `llama.cpp server binary for -ai local`)
	aiLocalModel := flag.String("ai-local-model", "", "GGUF model file for -ai local")
	aiContext := flag.Int("ai-context", 8192, "context size for -ai local")
	aiInstructions := flag.String("ai-instructions", "", `instructions added to every prompt the assistant sends, e.g. "Answer directly; don't reason at length." (\n for a new line)`)
	aiInstructionsFile := flag.String("ai-instructions-file", "", "file of instructions added to every prompt the assistant sends (after -ai-instructions)")
	imageAPI := flag.String("image-api", "", "separate OpenAI-format image API base URL ending in /v1; empty disables vignette generation")
	imageModel := flag.String("image-model", "", "image model ID (required with image-api; independent of ai-model)")
	imageSize := flag.String("image-size", "1024x1024", "image size supported by the image model")
	imageResponseFormat := flag.String("image-response-format", "", "empty for provider default; b64_json for gateways that require it")
	pushKey := flag.String("push-key", "", "VAPID key file for web push notifications (default: vapid.pem next to the -data file); created if missing")
	noPush := flag.Bool("no-push", false, "turn web push notifications off")
	playFrom := flag.String("playtest-from", "", "continue a finished single-game playtest in a NEW output directory; source is read-only")
	playPlayers := flag.Int("playtest-players", 0, "generate a disposable load scenario with 1–100 player commands; 0 uses the scenario unchanged")
	playConcurrency := flag.Int("playtest-concurrency", 4, "concurrent AI player requests (1–16); commander submits first")
	playScenario := flag.String("playtest-scenario", "st-ines", "scenario ID for disposable AI games (st-ines, crossroads, or wayward-tollhouse)")
	playDir := flag.String("playtest", "", "run AI players against the AI referee in a NEW output directory, then exit; never opens -data")
	playGames := flag.Int("playtest-games", 2, "number of disposable AI games (1–50)")
	playTurns := flag.Int("playtest-turns", 4, "maximum turns per AI game (1–50)")
	playCalls := flag.Int("playtest-max-calls", 80, "maximum model completions across AI players and judge (HTTP retries may add requests)")
	playCallLimit := flag.Duration("playtest-call-timeout", 2*time.Minute, "maximum wait for one AI playtest completion, including provider retries")
	playLimit := flag.Duration("playtest-timeout", 20*time.Minute, "maximum wall time for the whole AI playtest")
	playContact := flag.Bool("playtest-contact", true, "start every second AI game with a known enemy nearby to exercise combat")
	proxy := flag.Bool("proxy", false, "running behind a reverse proxy or tunnel that terminates HTTPS (trusts X-Forwarded-Proto and X-Forwarded-For)")
	flag.String("config", defaultConfig, "settings file: the flags above as name = value lines, plus ai-key, ai-key-file, image-key, image-key-file, host-password, and push-contact (see async-wargame.example.conf)")
	configFile, err := loadConfig(flag.CommandLine, os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	flag.Parse()
	if configFile != "" {
		log.Printf("settings from %s", configFile)
	}

	sources := []fs.FS{content.FS}
	if *modsDir != "" {
		sources = append(sources, os.DirFS(*modsDir))
	}
	cat, err := game.LoadCatalog(sources...)
	if err != nil {
		log.Fatalf("loading modules and scenarios: %v", err)
	}
	if *aiRetries < 0 || *aiRetries > 2 {
		log.Fatal("ai-retries must be 0–2")
	}
	if *aiConcurrency < 1 || *aiConcurrency > 16 {
		log.Fatal("ai-concurrency must be 1–16")
	}
	if *playDir != "" {
		if *aiFlag == "" || (*playLimit <= 0 || *playCallLimit <= 0) {
			log.Fatal("playtest needs -ai and a positive timeout")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, *playLimit)
		defer cancel()
		instructions, err := aiInstructionText(*aiInstructions, *aiInstructionsFile)
		if err != nil {
			log.Fatal(err)
		}
		client, local, err := startAI(ctx, *aiFlag, *aiModel, *aiLlama, *aiLocalModel, *aiContext, instructions, *aiEffort)
		if err != nil {
			log.Fatal(err)
		}
		if local != nil {
			defer local.Stop()
		}
		summary, err := playtest.Run(ctx, cat, client, playtest.Config{From: *playFrom, Players: *playPlayers, Concurrency: *playConcurrency, CallTimeout: *playCallLimit, Scenario: *playScenario, Directory: *playDir, Games: *playGames, Turns: *playTurns, MaxCalls: *playCalls, Contact: *playContact})
		log.Printf("playtest results: %s/summary.json (%d completions, %d tokens)", *playDir, summary.Calls, summary.Tokens)
		if err != nil {
			log.Printf("playtest found failures: %v", err)
			if local != nil {
				local.Stop()
			}
			os.Exit(1)
		}
		return
	}
	store, err := game.OpenStore(*dataPath, cat)
	if err != nil {
		log.Fatalf("opening campaign state: %v", err)
	}
	// The host password comes from the environment so it stays out of the process list.
	opts := web.Options{TrustProxy: *proxy, HostPassword: os.Getenv("AW_HOST_PASSWORD")}
	if !*noPush {
		path := *pushKey
		if path == "" {
			path = filepath.Join(filepath.Dir(*dataPath), "vapid.pem")
		}
		// Push services may contact this address about problems with our pushes.
		contact := os.Getenv("AW_PUSH_CONTACT")
		if contact == "" {
			contact = "mailto:webpush@example.com"
			log.Printf("AW_PUSH_CONTACT is not set: set it to a mailto: or https: address push services can reach you at")
		}
		sender, err := push.LoadOrCreate(path, contact)
		if err != nil {
			log.Fatalf("web push key: %v", err)
		}
		opts.Push = sender
	}
	if opts.HostPassword == "" {
		log.Printf("AW_HOST_PASSWORD is not set: anyone who can reach this server can create rooms")
	}
	srv, err := web.New(store, opts)
	if err != nil {
		log.Fatalf("building web server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go store.RunClock(30*time.Second, ctx.Done()) // locks turns on schedule; lets the AI stand in for an absent referee

	if *aiFlag != "" {
		instructions, err := aiInstructionText(*aiInstructions, *aiInstructionsFile)
		if err != nil {
			log.Fatalf("AI: %v", err)
		}
		client, local, err := startAI(ctx, *aiFlag, *aiModel, *aiLlama, *aiLocalModel, *aiContext, instructions, *aiEffort)
		if err != nil {
			log.Fatalf("AI: %v", err)
		}
		if local != nil {
			defer local.Stop()
			go func() {
				select {
				case <-local.Exited():
					log.Printf("AI: the local model server stopped; the assistant can't run until this server restarts")
				case <-ctx.Done():
				}
			}()
		}
		srv.SetAI(client)
		go (&assist.Worker{Store: store, Client: client, Concurrency: *aiConcurrency, Retries: *aiRetries}).Run(ctx)
		log.Printf("AI assistant: %s via %s", client.Model(), client.Provider())
		if in := client.Instructions(); in != "" {
			log.Printf("AI instructions added to every prompt: %q", in)
		}
	}

	if *imageAPI != "" {
		client, err := imageai.New(imageai.Config{BaseURL: *imageAPI, Model: *imageModel, APIKey: os.Getenv("AW_IMAGE_KEY"), Size: *imageSize, ResponseFormat: *imageResponseFormat})
		if err != nil {
			log.Fatalf("image AI: %v", err)
		}
		srv.SetImageAI(client)
		go (&imagework.Worker{Store: store, Client: client}).Run(ctx)
		log.Printf("Vignette images: %s via %s", client.Model(), client.Provider())
	} else if *imageModel != "" {
		log.Fatal("image-model needs a separate image-api")
	}

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shut) // live update streams are cut; browsers reconnect on their own
	}()
	log.Printf("campaign server listening on http://%s", *addr)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// startAI sets up the assistant's model from the -ai flags.
func startAI(ctx context.Context, which, model, llama, localModel string, contextSize int, instructions, effort string) (*ai.Client, *ai.Local, error) {
	key := os.Getenv("AW_AI_KEY")
	if key == "" {
		key = os.Getenv("OPENCODE_API_KEY") // what OpenCode's own tools use
	}
	if !slices.Contains([]string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"}, effort) {
		return nil, nil, fmt.Errorf("unknown ai-reasoning-effort %q", effort)
	}
	cfg := ai.Config{ReasoningEffort: effort, BaseURL: which, Model: model, APIKey: key, Name: which, Instructions: instructions}
	var local *ai.Local
	switch p, ok := ai.Presets[which]; {
	case ok:
		cfg.BaseURL, cfg.Name = p.BaseURL, p.Name
		if cfg.Model == "" {
			cfg.Model = p.Model
		}
		if p.NeedsKey && cfg.APIKey == "" {
			return nil, nil, fmt.Errorf("-ai %s needs an API key in AW_AI_KEY (or OPENCODE_API_KEY)", which)
		}
		if p.Session {
			cfg.Session = ai.NewSessionID()
		}
	case which == "local":
		if localModel == "" {
			return nil, nil, fmt.Errorf("-ai local needs -ai-local-model pointing at a GGUF file")
		}
		log.Printf("AI: starting %s with %s (loading can take a while)...", llama, localModel)
		loadCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		l, err := ai.StartLocal(loadCtx, llama, localModel, contextSize, nil)
		if err != nil {
			return nil, nil, err
		}
		local = l
		cfg.BaseURL, cfg.Name, cfg.Model = l.BaseURL, "a model running on this server", filepath.Base(localModel)
	default:
		u, err := url.Parse(which)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, nil, fmt.Errorf(`-ai is "opencode", "ollama", "local", or a base URL like http://127.0.0.1:1234/v1`)
		}
		if cfg.Model == "" {
			return nil, nil, fmt.Errorf("-ai with a URL needs -ai-model")
		}
		cfg.Name = u.Host
		if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1" {
			cfg.Name = "a model running on this server"
		}
	}
	client := ai.New(cfg)
	// Check the model exists, if the server will say.
	check, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if ids, err := client.Models(check); err == nil && len(ids) > 0 && !slices.Contains(ids, cfg.Model) {
		log.Printf("AI: warning: %s doesn't list a model called %q; it lists: %s", cfg.Name, cfg.Model, strings.Join(ids, ", "))
	}
	return client, local, nil
}

// aiInstructionText joins the operator's instructions for the assistant:
// the setting (where \n starts a new line) and then the file, if any.
func aiInstructionText(text, file string) (string, error) {
	parts := []string{strings.ReplaceAll(strings.TrimSpace(text), `\n`, "\n")}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("-ai-instructions-file: %w", err)
		}
		parts = append(parts, strings.TrimSpace(string(b)))
	}
	out := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if len(out) > 8000 {
		return "", fmt.Errorf("the AI instructions are %d characters; keep them under 8000", len(out))
	}
	return out, nil
}
