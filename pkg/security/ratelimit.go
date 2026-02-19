package security

import (
	"sync"
	"time"
)

// RateLimiter begrenzt die Nachrichten pro Nutzer (einfaches Sliding-Window)
type RateLimiter struct {
	mu       sync.Mutex
	windows  map[string][]time.Time
	maxCount int
	window   time.Duration
}

// NewRateLimiter erstellt einen neuen Rate Limiter
// maxCount: maximale Nachrichten pro Zeitfenster
// window: Zeitfenster (z.B. time.Minute)
func NewRateLimiter(maxCount int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		windows:  make(map[string][]time.Time),
		maxCount: maxCount,
		window:   window,
	}
	// Aufräum-Goroutine
	go rl.cleanup()
	return rl
}

// Allow prüft ob ein Nutzer eine weitere Nachricht senden darf
func (r *RateLimiter) Allow(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Alte Einträge entfernen
	times := r.windows[userID]
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.maxCount {
		r.windows[userID] = valid
		return false
	}

	r.windows[userID] = append(valid, now)
	return true
}

// cleanup bereinigt die interne Map periodisch
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		cutoff := time.Now().Add(-r.window)
		for userID, times := range r.windows {
			var valid []time.Time
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(r.windows, userID)
			} else {
				r.windows[userID] = valid
			}
		}
		r.mu.Unlock()
	}
}
