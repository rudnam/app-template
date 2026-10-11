package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ClientIP returns the caller's IP. With trustProxy it reads the headers a
// reverse proxy sets; enable that only when the app is reachable solely
// through the proxy, or callers can forge their IP.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		// Cloudflare sets this to the visitor's IP, overwriting any value the
		// visitor sent.
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// The right-most entry was added by our own proxy (Caddy, an AWS
			// load balancer); entries further left come from the client.
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

// RateLimiter is a per-key token bucket held in memory. Each app instance
// keeps its own, so with N instances the effective limit is up to N times
// higher. That is fine for slowing down password guessing; if you need an
// exact global limit, back it with the database or Redis.
type RateLimiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter allows perWindow events per window, in bursts of up to perWindow.
func NewRateLimiter(perWindow int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		rate:    float64(perWindow) / window.Seconds(),
		burst:   float64(perWindow),
		buckets: map[string]*bucket{},
	}
}

// Allow reports whether key may act now, and uses up one token if so.
func (l *RateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 10_000 { // bound memory: drop idle buckets
			for k, old := range l.buckets {
				if now.Sub(old.last) > 10*time.Minute {
					delete(l.buckets, k)
				}
			}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
