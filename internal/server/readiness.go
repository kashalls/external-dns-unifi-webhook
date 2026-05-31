package server

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// ProbeFunc is the signature the health server invokes to decide whether the
// process should report ready. Returning a non-nil error fails the probe.
type ProbeFunc func(ctx context.Context) error

// cachedProbe wraps a ProbeFunc with a TTL-bounded result cache and
// singleflight deduplication. Multiple concurrent /readyz hits within the
// TTL share a single in-flight check; subsequent hits in that window return
// the memoised verdict without touching the upstream.
type cachedProbe struct {
	probe ProbeFunc
	ttl   time.Duration
	sf    singleflight.Group

	mu      sync.RWMutex
	lastAt  time.Time
	lastErr error
}

func newCachedProbe(probe ProbeFunc, ttl time.Duration) *cachedProbe {
	return &cachedProbe{probe: probe, ttl: ttl}
}

// Check returns the latest cached result if it is still within ttl, otherwise
// it runs the probe (deduped) and returns its result.
func (c *cachedProbe) Check(ctx context.Context) error {
	c.mu.RLock()
	fresh := !c.lastAt.IsZero() && time.Since(c.lastAt) < c.ttl
	cached := c.lastErr
	c.mu.RUnlock()
	if fresh {
		return cached
	}

	_, err, _ := c.sf.Do("probe", func() (any, error) {
		// Re-check inside the singleflight in case another caller refreshed
		// the cache while we were queued.
		c.mu.RLock()
		stillFresh := !c.lastAt.IsZero() && time.Since(c.lastAt) < c.ttl
		stillCached := c.lastErr
		c.mu.RUnlock()
		if stillFresh {
			return nil, stillCached
		}

		err := c.probe(ctx)

		c.mu.Lock()
		c.lastAt = time.Now()
		c.lastErr = err
		c.mu.Unlock()

		return nil, err
	})

	return err
}

// noopProbe is used when the caller does not wire a real probe — keeps
// /readyz returning 200 for backwards compatibility in tests.
func noopProbe(_ context.Context) error { return nil }
