package limiter

import "sync/atomic"

// GlobalLimiter tracks global rover client counts.
// It is used to enforce a server-wide limit on the number of concurrent rovers.
type GlobalLimiter struct {
	count     atomic.Int64
	maxCount  int
}

// NewGlobalLimiter creates a limiter with the given maximum.
func NewGlobalLimiter(maxClients int) *GlobalLimiter {
	return &GlobalLimiter{maxCount: maxClients}
}

// Count returns the current number of clients.
func (l *GlobalLimiter) Count() int64 {
	return l.count.Load()
}

// Max returns the configured maximum.
func (l *GlobalLimiter) Max() int {
	return l.maxCount
}

// Add increments the counter. Call when a rover connects.
func (l *GlobalLimiter) Add() {
	l.count.Add(1)
}

// Release decrements the counter. Call when a rover disconnects.
func (l *GlobalLimiter) Release() {
	l.count.Add(-1)
}

// AtCapacity returns true if the limit has been reached.
func (l *GlobalLimiter) AtCapacity() bool {
	return l.count.Load() >= int64(l.maxCount)
}