package httpapi

import (
	"sync"

	"golang.org/x/time/rate"
)

// limiter is a set of token buckets keyed by a string, held in memory.
type limiter struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter
	r  rate.Limit
	b  int
}

func newLimiter(r rate.Limit, b int) *limiter {
	return &limiter{m: map[string]*rate.Limiter{}, r: r, b: b}
}

// Allow takes one token for key. When the map passes 1000 keys, inserting a
// new key first evicts every bucket that has refilled to full.
func (l *limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.m[key]
	if !ok {
		if len(l.m) > 1000 {
			for k, v := range l.m {
				if v.Tokens() >= float64(l.b) {
					delete(l.m, k)
				}
			}
		}
		lim = rate.NewLimiter(l.r, l.b)
		l.m[key] = lim
	}
	return lim.Allow()
}
