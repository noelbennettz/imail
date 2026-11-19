// Package loginlimit bounds failed mail-authentication attempts.
//
// The listeners share one in-memory limiter. A key is the source address of the
// connection or the account being tried; after MaxFailures failures inside
// FailureWindow the key is refused for BlockDuration. Every refusal is reported
// to the client as the protocol's ordinary authentication failure, so a blocked
// reply never says whether the account exists. A global ceiling stops a
// distributed spray from turning into unlimited password hashing, and the key
// map is bounded so a spray cannot grow it without limit.
//
// Nothing here is persisted: a restart forgets the counters, which is
// documented for administrators.
package loginlimit

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/noelbennettz/imail/config"
)

// Options configures a Limiter. Zero values are replaced by DefaultOptions.
type Options struct {
	// MaxFailures is the number of failures within FailureWindow that blocks
	// a key.
	MaxFailures int
	// FailureWindow is how far back failures are counted.
	FailureWindow time.Duration
	// BlockDuration is how long a key stays blocked once it reaches
	// MaxFailures.
	BlockDuration time.Duration
	// MaxKeys bounds the number of tracked keys.
	MaxKeys int
	// GlobalMaxFailures is the ceiling of failures, over all keys, inside
	// GlobalWindow. Reaching it refuses every attempt until the window moves
	// on, which bounds the password hashing a spray can cause.
	GlobalMaxFailures int
	GlobalWindow      time.Duration
	// Now supplies the clock; tests replace it.
	Now func() time.Time
}

// DefaultOptions is the policy the mail listeners use: 5 failures in 15
// minutes block for 15 minutes, at most 100 failures a minute in total, and at
// most 4096 keys are remembered.
var DefaultOptions = Options{
	MaxFailures:       5,
	FailureWindow:     15 * time.Minute,
	BlockDuration:     15 * time.Minute,
	MaxKeys:           4096,
	GlobalMaxFailures: 100,
	GlobalWindow:      time.Minute,
}

// Default is the limiter shared by the SMTP, IMAP and POP3 listeners. Tests
// replace it with a limiter that uses a fake clock.
var Default = New(DefaultOptions)

const (
	addressPrefix = "ip:"
	accountPrefix = "acct:"
)

type entry struct {
	failures     []time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

// Limiter counts failures per source address and per account.
type Limiter struct {
	mu      sync.Mutex
	opts    Options
	entries map[string]*entry
	// global holds the failure timestamps of the whole server, newest last.
	global []time.Time
}

// New builds a limiter. Fields left at their zero value fall back to
// DefaultOptions.
func New(opts Options) *Limiter {
	defaults := DefaultOptions
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = defaults.MaxFailures
	}
	if opts.FailureWindow <= 0 {
		opts.FailureWindow = defaults.FailureWindow
	}
	if opts.BlockDuration <= 0 {
		opts.BlockDuration = defaults.BlockDuration
	}
	if opts.MaxKeys <= 0 {
		opts.MaxKeys = defaults.MaxKeys
	}
	if opts.GlobalMaxFailures <= 0 {
		opts.GlobalMaxFailures = defaults.GlobalMaxFailures
	}
	if opts.GlobalWindow <= 0 {
		opts.GlobalWindow = defaults.GlobalWindow
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Limiter{
		opts:    opts,
		entries: make(map[string]*entry),
	}
}

// Blocked reports whether an attempt for source and account must be refused
// before any password work happens. Either argument may be empty; empty keys
// are simply not tracked.
func (l *Limiter) Blocked(source, account string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.opts.Now()
	if l.globalBlockedLocked(now) {
		return true
	}
	for _, key := range keys(source, account) {
		if l.keyBlockedLocked(key, now) {
			return true
		}
	}
	return false
}

// Failure records one failed attempt for both keys.
func (l *Limiter) Failure(source, account string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.opts.Now()
	l.global = prune(l.global, now.Add(-l.opts.GlobalWindow))
	l.global = append(l.global, now)

	for _, key := range keys(source, account) {
		l.recordLocked(key, now)
	}
}

// Success clears the counters of both keys. The global ceiling is left alone:
// it counts failures, not successes.
func (l *Limiter) Success(source, account string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys(source, account) {
		delete(l.entries, key)
	}
}

// Keys returns the number of tracked keys; tests use it to prove the map is
// bounded.
func (l *Limiter) Keys() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func (l *Limiter) globalBlockedLocked(now time.Time) bool {
	l.global = prune(l.global, now.Add(-l.opts.GlobalWindow))
	return len(l.global) >= l.opts.GlobalMaxFailures
}

func (l *Limiter) keyBlockedLocked(key string, now time.Time) bool {
	tracked, ok := l.entries[key]
	if !ok {
		return false
	}
	if now.Before(tracked.blockedUntil) {
		return true
	}
	// The block expired: forget the old failures so the next window starts
	// clean instead of immediately re-blocking the key.
	if !tracked.blockedUntil.IsZero() {
		delete(l.entries, key)
	}
	return false
}

func (l *Limiter) recordLocked(key string, now time.Time) {
	tracked, ok := l.entries[key]
	if !ok {
		l.makeRoomLocked(now)
		tracked = &entry{}
		l.entries[key] = tracked
	}
	tracked.lastSeen = now
	tracked.failures = prune(tracked.failures, now.Add(-l.opts.FailureWindow))
	tracked.failures = append(tracked.failures, now)
	if len(tracked.failures) >= l.opts.MaxFailures {
		tracked.blockedUntil = now.Add(l.opts.BlockDuration)
	}
}

// makeRoomLocked keeps the map bounded. Expired entries go first; if the map is
// still full the least recently seen key is dropped, which can only lose
// counters, never allow extra password hashing.
func (l *Limiter) makeRoomLocked(now time.Time) {
	if len(l.entries) < l.opts.MaxKeys {
		return
	}
	for key, tracked := range l.entries {
		if now.Sub(tracked.lastSeen) > l.opts.FailureWindow && !now.Before(tracked.blockedUntil) {
			delete(l.entries, key)
			if len(l.entries) < l.opts.MaxKeys {
				return
			}
		}
	}
	if len(l.entries) < l.opts.MaxKeys {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, tracked := range l.entries {
		if oldestKey == "" || tracked.lastSeen.Before(oldest) {
			oldestKey, oldest = key, tracked.lastSeen
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

// keys lists the keys an attempt is counted against.
func keys(source, account string) []string {
	var out []string
	if source = strings.TrimSpace(source); source != "" {
		out = append(out, addressPrefix+strings.ToLower(source))
	}
	if account = strings.TrimSpace(account); account != "" {
		out = append(out, accountPrefix+strings.ToLower(account))
	}
	return out
}

func prune(times []time.Time, cutoff time.Time) []time.Time {
	first := 0
	// A failure exactly one window old no longer counts: the window is
	// half-open, so a key unblocks precisely BlockDuration after its last
	// counted failure.
	for first < len(times) && !times[first].After(cutoff) {
		first++
	}
	if first == 0 {
		return times
	}
	return append(times[:0], times[first:]...)
}

// SourceKey derives the source-address part of a limiter key from a connection
// address: the host without port or IPv6 zone. It is empty when the address is
// nil, which callers treat as "unknown".
func SourceKey(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	if zone := strings.LastIndex(host, "%"); zone >= 0 {
		host = host[:zone]
	}
	return strings.Trim(host, "[]")
}

// SourceKeyConn is SourceKey for a connection that may be nil.
func SourceKeyConn(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	return SourceKey(conn.RemoteAddr())
}

// Blocked reports whether the shared limiter currently refuses an attempt. The
// administrator's switch short-circuits it: with the setting off no attempt is
// ever refused and no counter is kept.
func Blocked(source, account string) bool {
	return config.LoginLimiterEnabled() && Default.Blocked(source, account)
}

// Failure records a failed attempt on the shared limiter.
func Failure(source, account string) {
	if config.LoginLimiterEnabled() {
		Default.Failure(source, account)
	}
}

// Success clears the shared limiter's counters for these keys.
func Success(source, account string) {
	Default.Success(source, account)
}
