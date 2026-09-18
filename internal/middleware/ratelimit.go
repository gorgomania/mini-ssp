package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const clientTTL = 3 * time.Minute

type perIPLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	rps     rate.Limit
	burst   int
}

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func (p *perIPLimiter) get(ip string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[ip]
	if !ok {
		e = &ipEntry{limiter: rate.NewLimiter(p.rps, p.burst)}
		p.entries[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

func (p *perIPLimiter) cleanup() {
	t := time.NewTicker(clientTTL)
	for range t.C {
		cutoff := time.Now().Add(-clientTTL)
		p.mu.Lock()
		for ip, e := range p.entries {
			if e.lastSeen.Before(cutoff) {
				delete(p.entries, ip)
			}
		}
		p.mu.Unlock()
	}
}

// RateLimit returns a middleware that limits requests per source IP to rps/s
// with the given burst. Each IP gets its own token bucket.
// Excess requests get 429 Too Many Requests.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	p := &perIPLimiter{
		entries: make(map[string]*ipEntry),
		rps:     rate.Limit(rps),
		burst:   burst,
	}
	go p.cleanup()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !p.get(ip).Allow() {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
