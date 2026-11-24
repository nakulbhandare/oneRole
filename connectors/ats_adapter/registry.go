package ats_adapter

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// TokenBucket implements a standard thread-safe token bucket rate limiter.
type TokenBucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

func NewTokenBucket(ratePerSec float64, capacity int) *TokenBucket {
	return &TokenBucket{
		rate:       ratePerSec,
		capacity:   float64(capacity),
		tokens:     float64(capacity),
		lastUpdate: time.Now(),
	}
}

func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(tb.lastUpdate).Seconds()
		tb.lastUpdate = now
		tb.tokens += elapsed * tb.rate
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}

		if tb.tokens >= 1.0 {
			tb.tokens -= 1.0
			tb.mu.Unlock()
			return nil
		}

		// Calculate sleep time until next token
		missing := 1.0 - tb.tokens
		sleepSec := missing / tb.rate
		tb.mu.Unlock()

		select {
		case <-time.After(time.Duration(sleepSec * float64(time.Second))):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type AdapterRegistry struct {
	mu           sync.RWMutex
	adapters     []ATSAdapter
	rateLimiters map[string]*TokenBucket
	semaphores   map[string]chan struct{}
}

func NewEmptyRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		adapters:     make([]ATSAdapter, 0),
		rateLimiters: make(map[string]*TokenBucket),
		semaphores:   make(map[string]chan struct{}),
	}
}

func NewDefaultRegistry() *AdapterRegistry {
	r := &AdapterRegistry{
		adapters:     make([]ATSAdapter, 0),
		rateLimiters: make(map[string]*TokenBucket),
		semaphores:   make(map[string]chan struct{}),
	}

	httpClient := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Register prioritized adapters: 1. SmartRecruiters, 2. Greenhouse, 3. Lever, 4. Ashby, 5. Workday
	r.Register(NewSmartRecruitersAdapter(httpClient), 20.0, 30) // 20 req/s, 30 concurrent
	r.Register(NewGreenhouseAdapter(httpClient), 15.0, 25)      // 15 req/s, 25 concurrent
	r.Register(NewLeverAdapter(httpClient), 15.0, 25)           // 15 req/s, 25 concurrent
	r.Register(NewAshbyAdapter(httpClient), 10.0, 15)           // 10 req/s, 15 concurrent
	r.Register(NewWorkdayAdapter(httpClient), 5.0, 10)          // 5 req/s, 10 concurrent

	return r
}

func (r *AdapterRegistry) Register(adapter ATSAdapter, rateLimitPerSec float64, maxConcurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.adapters = append(r.adapters, adapter)
	p := adapter.Provider()

	if rateLimitPerSec <= 0 {
		rateLimitPerSec = 10.0
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}

	r.rateLimiters[p] = NewTokenBucket(rateLimitPerSec, maxConcurrency*2)
	r.semaphores[p] = make(chan struct{}, maxConcurrency)
}

func (r *AdapterRegistry) Match(jobURL string) (ATSAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, a := range r.adapters {
		if a.CanHandle(jobURL) {
			return a, true
		}
	}
	return nil, false
}

func (r *AdapterRegistry) Acquire(ctx context.Context, provider string) (func(), error) {
	r.mu.RLock()
	limiter := r.rateLimiters[provider]
	sem := r.semaphores[provider]
	r.mu.RUnlock()

	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait: %w", err)
		}
	}

	if sem != nil {
		select {
		case sem <- struct{}{}:
			return func() { <-sem }, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return func() {}, nil
}
