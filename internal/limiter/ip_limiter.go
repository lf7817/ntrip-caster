// Package limiter provides per-IP concurrent connection limiting.
package limiter

import "sync"

// IPLimiter tracks per-IP concurrent connection counts and enforces a limit.
type IPLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	limit  int
}

// NewIPLimiter creates a limiter with the given per-IP maximum.
func NewIPLimiter(limit int) *IPLimiter {
	return &IPLimiter{
		counts: make(map[string]int),
		limit:  limit,
	}
}

// Allow checks whether ip is below the limit. If allowed, it increments the
// counter and returns true. The caller must call Release when the connection
// closes.
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[ip] >= l.limit {
		return false
	}
	l.counts[ip]++
	return true
}

// Release decrements the counter for ip.
func (l *IPLimiter) Release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[ip]--
	if l.counts[ip] <= 0 {
		delete(l.counts, ip)
	}
}
