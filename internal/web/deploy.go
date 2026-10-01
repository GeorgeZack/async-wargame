package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// https reports whether the browser reached us over HTTPS, directly or
// through a trusted reverse proxy.
func (s *Server) https(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.opts.TrustProxy && strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https")
}

// clientIP is the browser's address. Behind a trusted proxy that's the last
// address the proxy added to X-Forwarded-For; earlier entries can be forged.
func (s *Server) clientIP(r *http.Request) string {
	if s.opts.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// failGuard blocks an address after too many failures within a window.
type failGuard struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
}

func newFailGuard(max int, window time.Duration) *failGuard {
	return &failGuard{max: max, window: window, fails: map[string][]time.Time{}}
}

func (g *failGuard) recent(ip string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range g.fails[ip] {
		if now.Sub(t) < g.window {
			keep = append(keep, t)
		}
	}
	if keep == nil {
		delete(g.fails, ip)
	} else {
		g.fails[ip] = keep
	}
	return keep
}

func (g *failGuard) blocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.recent(ip, time.Now())) >= g.max
}

func (g *failGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	g.fails[ip] = append(g.recent(ip, now), now)
}
