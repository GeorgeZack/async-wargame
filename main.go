// Command async-wargame runs a campaign server: one binary serving the
// browser client, room membership, and the campaign record.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	_ "time/tzdata" // turn schedules use named time zones; don't depend on the host's zoneinfo

	"asyncwargame/content"
	"asyncwargame/internal/game"
	"asyncwargame/internal/push"
	"asyncwargame/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address (use :8080 to accept LAN connections)")
	dataPath := flag.String("data", "data/campaigns.json", "campaign state file")
	modsDir := flag.String("modules", "", "extra directory with modules/*.json and scenarios/*.json to load alongside the built-in ones")
	pushKey := flag.String("push-key", "", "VAPID key file for web push notifications (default: vapid.pem next to the -data file); created if missing")
	noPush := flag.Bool("no-push", false, "turn web push notifications off")
	proxy := flag.Bool("proxy", false, "running behind a reverse proxy or tunnel that terminates HTTPS (trusts X-Forwarded-Proto and X-Forwarded-For)")
	flag.Parse()

	sources := []fs.FS{content.FS}
	if *modsDir != "" {
		sources = append(sources, os.DirFS(*modsDir))
	}
	cat, err := game.LoadCatalog(sources...)
	if err != nil {
		log.Fatalf("loading modules and scenarios: %v", err)
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

	go store.RunClock(30*time.Second, nil) // locks turns on schedule; lets the AI stand in for an absent referee

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("campaign server listening on http://%s", *addr)
	log.Fatal(hs.ListenAndServe())
}
