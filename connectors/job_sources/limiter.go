package job_sources

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type limiterState struct {
	mu         sync.Mutex
	windowFrom time.Time
	used       int
	concurrent chan struct{}
}

// ProcessRateLimiter limits requests per source within one process. Production
// deployments must also ensure one database-leased sync owner per source, or
// inject a shared limiter when source leases can overlap across replicas.
type ProcessRateLimiter struct {
	mu     sync.Mutex
	states map[string]*limiterState
}

func NewProcessRateLimiter() *ProcessRateLimiter {
	return &ProcessRateLimiter{states: make(map[string]*limiterState)}
}

func (l *ProcessRateLimiter) Acquire(ctx context.Context, source SourceConfig) (func(), error) {
	if l == nil || source.ID == "" || source.RequestsPerWindow <= 0 || source.Window <= 0 || source.MaxConcurrency <= 0 {
		return nil, fmt.Errorf("%w: invalid rate limiter settings", ErrInvalidConfig)
	}
	key := fmt.Sprintf("%s:%d", source.ID, source.ConfigVersion)
	state := l.state(key, source.MaxConcurrency)
	select {
	case state.concurrent <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for {
		now := time.Now()
		state.mu.Lock()
		if state.windowFrom.IsZero() || now.Sub(state.windowFrom) >= source.Window {
			state.windowFrom = now
			state.used = 0
		}
		if state.used < source.RequestsPerWindow {
			state.used++
			state.mu.Unlock()
			return func() { <-state.concurrent }, nil
		}
		wait := source.Window - now.Sub(state.windowFrom)
		state.mu.Unlock()
		if wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			<-state.concurrent
			return nil, ctx.Err()
		}
	}
}

func (l *ProcessRateLimiter) state(key string, maxConcurrency int) *limiterState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing := l.states[key]; existing != nil {
		return existing
	}
	state := &limiterState{concurrent: make(chan struct{}, maxConcurrency)}
	l.states[key] = state
	return state
}

var _ RateLimiter = (*ProcessRateLimiter)(nil)
