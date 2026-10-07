package botpanel

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// A login form guarded only by a comparison is a brute-force target. The
// admin Telegram ID is not a secret - it is printed in bot logs, appears in
// webhook payloads and is guessable from any chat the bot is in - so before
// this limiter existed an attacker who guessed four digits could simply keep
// trying. Every attempt is now counted per source address and a run of
// failures locks that source out with a backoff that grows each time.

// limiter backs off per source address. It is safe for concurrent use: the
// panel handles logins from many connections at once.
type limiter struct {
	mu      sync.Mutex
	entries map[string]*attempt

	// threshold is how many consecutive failures trigger a lockout.
	threshold int
	// base is the first lockout; each further one doubles it.
	base time.Duration
	// cap bounds the backoff so a real operator who mistypes still gets back in.
	cap time.Duration
	// maxEntries bounds memory. A flood of distinct source addresses is
	// answered from a shared default until the table is swept.
	maxEntries int
	// window is how long a source with no failures is remembered.
	window time.Duration
	// now is injectable so tests do not have to sleep.
	now func() time.Time
}

type attempt struct {
	failures int
	// lockout is the length currently in force, so the next one can grow from
	// it instead of restarting at the base every time.
	lockout     time.Duration
	lockedUntil time.Time
	seen        time.Time
}

// defaultLimiter is tuned for an admin panel: a handful of tries, then a wait
// that doubles from five seconds up to fifteen minutes.
func defaultLimiter() *limiter {
	return &limiter{
		entries:    map[string]*attempt{},
		threshold:  5,
		base:       5 * time.Second,
		cap:        15 * time.Minute,
		maxEntries: 4096,
		window:     30 * time.Minute,
		now:        time.Now,
	}
}

// sourceAddress identifies the caller.
//
// It deliberately uses RemoteAddr and ignores X-Forwarded-For. Behind an
// untrusted proxy a forwarded header is whatever the client typed, so
// honouring it would let an attacker mint a fresh identity per request and
// the limiter would never count anything. An operator who fronts the bot
// should keep the panel off the public internet or terminate the limit at
// their own edge.
func sourceAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allow reports whether this source may attempt a login, and if not how long
// it must wait.
func (l *limiter) allow(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked()
	entry, ok := l.entries[key]
	if !ok {
		return 0, true
	}
	if remaining := entry.lockedUntil.Sub(l.now()); remaining > 0 {
		return remaining, false
	}
	return 0, true
}

// fail records a bad attempt, locking the source out once it has failed
// threshold times. The lockout doubles on every further run of failures.
func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked()
	if len(l.entries) >= l.maxEntries {
		// Refuse to grow the table for an attacker spraying addresses. The
		// entry is still recorded when there is room; otherwise the source
		// simply gets the shared default backoff on its next check.
		if _, known := l.entries[key]; !known {
			return
		}
	}
	entry, ok := l.entries[key]
	if !ok {
		entry = &attempt{}
		l.entries[key] = entry
	}
	now := l.now()
	entry.failures++
	entry.seen = now
	if entry.failures < l.threshold {
		return
	}
	entry.failures = 0
	switch {
	case entry.lockout < l.base:
		entry.lockout = l.base
	case entry.lockout < l.cap:
		entry.lockout *= 2
	}
	if entry.lockout > l.cap {
		entry.lockout = l.cap
	}
	entry.lockedUntil = now.Add(entry.lockout)
}

// succeed clears the record for a source that just authenticated.
func (l *limiter) succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// backoff grows the previous lockout from base up to cap. It is called with
// the length already in force, so the first lockout is exactly base.
func (l *limiter) backoff(previous time.Duration) time.Duration {
	if previous <= 0 {
		return l.base
	}
	next := previous * 2
	if next > l.cap {
		return l.cap
	}
	return next
}

// sweepLocked drops entries that have been idle past the window, or whose
// lockout has expired, so the table cannot grow without bound.
func (l *limiter) sweepLocked() {
	now := l.now()
	for key, entry := range l.entries {
		if now.Sub(entry.seen) > l.window && !entry.lockedUntil.After(now) {
			delete(l.entries, key)
		}
	}
}
