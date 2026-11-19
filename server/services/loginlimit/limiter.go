// Package loginlimit bounds failed mail-authentication attempts.
//
// The listeners share one in-memory limiter. The primary control is the source
// address: after SourceMaxFailures failures inside SourceWindow the address is
// refused for SourceBlock, whatever account it tries. Accounts are a secondary
// control and are deliberately harder to block: an account is only blocked when
// AccountMaxFailures failures inside AccountWindow come from at least
// AccountMinSources distinct addresses. One source can therefore never lock an
// account out on its own, which keeps a targeted lockout from being a cheap
// denial of service.
//
// There is no global ceiling that refuses attempts. A distributed spray is
// allowed to keep trying, but the password work is bounded by a semaphore
// around the PBKDF2 verification, so a correct login is only delayed, never
// refused, by a spray. The global failure counter feeds logging and alerting
// only.
//
// Every refusal is reported to the client as the protocol's ordinary
// authentication failure, so a blocked reply never says whether the account
// exists. Nothing here is persisted: a restart forgets the counters, which is
// documented for administrators.
package loginlimit

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/noelbennettz/imail/config"
	log "github.com/sirupsen/logrus"
)

// Realm namespaces the per-account counters. The per-source counter is shared by
// every realm, so one address cannot dodge a block by switching from SMTP to
// webmail; account failures do not leak across planes, so a spray at the
// webmail login cannot lock an administrator's console login.
type Realm string

const (
	// RealmMail covers SMTP, IMAP and POP3 authentication.
	RealmMail Realm = "mail"
	// RealmWeb covers the user webmail login.
	RealmWeb Realm = "web"
	// RealmAdmin covers the administrator console login.
	RealmAdmin Realm = "admin"
)

// Options configures a Limiter. Zero values are replaced by DefaultOptions.
type Options struct {
	// SourceMaxFailures is the number of failures within SourceWindow that
	// blocks a source address.
	SourceMaxFailures int
	// SourceWindow is how far back source failures are counted.
	SourceWindow time.Duration
	// SourceBlock is how long a source address stays blocked.
	SourceBlock time.Duration
	// AccountMaxFailures is the number of failures within AccountWindow that,
	// together with AccountMinSources distinct sources, blocks an account.
	AccountMaxFailures int
	// AccountWindow is how far back account failures are counted.
	AccountWindow time.Duration
	// AccountBlock is how long a blocked account stays blocked.
	AccountBlock time.Duration
	// AccountMinSources is the number of distinct source addresses that must
	// have contributed before an account can be blocked at all. It must be at
	// least 2; one source alone never locks an account.
	AccountMinSources int
	// MaxKeys bounds the number of tracked keys.
	MaxKeys int
	// AlertFailures is the failure count inside AlertWindow that triggers
	// OnAlert. Reaching it never refuses an attempt: the counter exists for
	// logging and alerting only.
	AlertFailures int
	AlertWindow   time.Duration
	// OnAlert is called outside the limiter lock when AlertFailures failures
	// happened inside AlertWindow. It fires at most once per window.
	OnAlert func(failures int, window time.Duration)
	// Now supplies the clock; tests replace it.
	Now func() time.Time
}

// DefaultOptions is the policy the mail listeners and the web login use: 5
// failures in 15 minutes block a source for 15 minutes; an account needs 25
// failures from at least 3 distinct sources inside 15 minutes before it is
// blocked for 15 minutes; at most 4096 keys are remembered; 100 failures a
// minute produce one alert line.
var DefaultOptions = Options{
	SourceMaxFailures:  5,
	SourceWindow:       15 * time.Minute,
	SourceBlock:        15 * time.Minute,
	AccountMaxFailures: 25,
	AccountWindow:      15 * time.Minute,
	AccountBlock:       15 * time.Minute,
	AccountMinSources:  3,
	MaxKeys:            4096,
	AlertFailures:      100,
	AlertWindow:        time.Minute,
}

// Default is the limiter shared by the SMTP, IMAP, POP3 and HTTP listeners.
// Tests replace it with a limiter that uses a fake clock.
var Default = New(DefaultOptions)

const (
	addressPrefix = "ip:"
	accountSuffix = ":acct:"
)

// failure is one failed attempt against an account, tagged with the source
// address it came from so the distinct-source rule can be evaluated.
type failure struct {
	at     time.Time
	source string
}

type entry struct {
	// sourceFailures holds the timestamps of an address key.
	sourceFailures []time.Time
	// accountFailures holds the tagged failures of an account key.
	accountFailures []failure
	blockedUntil    time.Time
	lastSeen        time.Time
}

// Limiter counts failures per source address and, more conservatively, per
// account.
type Limiter struct {
	mu      sync.Mutex
	opts    Options
	entries map[string]*entry
	// global holds the failure timestamps of the whole server, newest last.
	// They feed logging and alerting; they never refuse an attempt.
	global      []time.Time
	lastAlertAt time.Time
}

// New builds a limiter. Fields left at their zero value fall back to
// DefaultOptions.
func New(opts Options) *Limiter {
	defaults := DefaultOptions
	if opts.SourceMaxFailures <= 0 {
		opts.SourceMaxFailures = defaults.SourceMaxFailures
	}
	if opts.SourceWindow <= 0 {
		opts.SourceWindow = defaults.SourceWindow
	}
	if opts.SourceBlock <= 0 {
		opts.SourceBlock = defaults.SourceBlock
	}
	if opts.AccountMaxFailures <= 0 {
		opts.AccountMaxFailures = defaults.AccountMaxFailures
	}
	if opts.AccountWindow <= 0 {
		opts.AccountWindow = defaults.AccountWindow
	}
	if opts.AccountBlock <= 0 {
		opts.AccountBlock = defaults.AccountBlock
	}
	if opts.AccountMinSources < 2 {
		opts.AccountMinSources = defaults.AccountMinSources
	}
	if opts.MaxKeys <= 0 {
		opts.MaxKeys = defaults.MaxKeys
	}
	if opts.AlertFailures <= 0 {
		opts.AlertFailures = defaults.AlertFailures
	}
	if opts.AlertWindow <= 0 {
		opts.AlertWindow = defaults.AlertWindow
	}
	if opts.OnAlert == nil {
		opts.OnAlert = LogAlert
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Limiter{
		opts:    opts,
		entries: make(map[string]*entry),
	}
}

// LogAlert is the production alert sink: one warning line per window.
func LogAlert(failures int, window time.Duration) {
	log.Warnf("Failed sign-in alert: %d failures in %s across many sources; password checks stay bounded and per-source blocking stays active", failures, window)
}

// Blocked reports whether an attempt for source and account must be refused
// before any password work happens. Either argument may be empty; empty keys are
// simply not tracked.
func (l *Limiter) Blocked(realm Realm, source, account string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.opts.Now()
	if l.keyBlockedLocked(addressKey(source), now) {
		return true
	}
	return l.keyBlockedLocked(accountKey(realm, account), now)
}

// BlockedSource reports whether the source address alone is refused. It lets a
// handler reject a blocked address before it even reads the account from the
// database.
func (l *Limiter) BlockedSource(source string) bool {
	return l.Blocked("", source, "")
}

// Failure records one failed attempt for the source and the account.
func (l *Limiter) Failure(realm Realm, source, account string) {
	l.mu.Lock()
	now := l.opts.Now()

	l.global = prune(l.global, now.Add(-l.opts.AlertWindow))
	l.global = append(l.global, now)
	alert := len(l.global) >= l.opts.AlertFailures && now.Sub(l.lastAlertAt) >= l.opts.AlertWindow
	if alert {
		l.lastAlertAt = now
	}
	count := len(l.global)

	if key := addressKey(source); key != "" {
		l.recordSourceLocked(key, now)
	}
	if key := accountKey(realm, account); key != "" {
		l.recordAccountLocked(key, source, now)
	}
	l.mu.Unlock()

	if alert && l.opts.OnAlert != nil {
		l.opts.OnAlert(count, l.opts.AlertWindow)
	}
}

// Success clears the account counter of the authenticated realm and the source
// counter, so an ordinary typo followed by a successful login does not creep
// towards a block. The global counter is left alone: it counts failures, not
// successes.
func (l *Limiter) Success(realm Realm, source, account string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.opts.Now()
	if key := accountKey(realm, account); key != "" {
		delete(l.entries, key)
	}
	if key := addressKey(source); key != "" {
		if tracked, ok := l.entries[key]; ok && !now.Before(tracked.blockedUntil) {
			tracked.sourceFailures = nil
		}
	}
}

// Keys returns the number of tracked keys; tests use it to prove the map is
// bounded.
func (l *Limiter) Keys() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// GlobalFailures returns the number of failures inside the alert window. It is
// observability only and never gates an attempt.
func (l *Limiter) GlobalFailures() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(prune(l.global, l.opts.Now().Add(-l.opts.AlertWindow)))
}

func (l *Limiter) keyBlockedLocked(key string, now time.Time) bool {
	if key == "" {
		return false
	}
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

func (l *Limiter) recordSourceLocked(key string, now time.Time) {
	tracked := l.trackedLocked(key, now)
	tracked.sourceFailures = prune(tracked.sourceFailures, now.Add(-l.opts.SourceWindow))
	tracked.sourceFailures = append(tracked.sourceFailures, now)
	if len(tracked.sourceFailures) >= l.opts.SourceMaxFailures {
		tracked.blockedUntil = now.Add(l.opts.SourceBlock)
	}
}

func (l *Limiter) recordAccountLocked(key, source string, now time.Time) {
	tracked := l.trackedLocked(key, now)
	cutoff := now.Add(-l.opts.AccountWindow)
	kept := tracked.accountFailures[:0]
	for _, failed := range tracked.accountFailures {
		if failed.at.After(cutoff) {
			kept = append(kept, failed)
		}
	}
	tracked.accountFailures = append(kept, failure{at: now, source: strings.ToLower(strings.TrimSpace(source))})
	if len(tracked.accountFailures) < l.opts.AccountMaxFailures {
		return
	}
	if len(distinctSources(tracked.accountFailures)) < l.opts.AccountMinSources {
		return
	}
	tracked.blockedUntil = now.Add(l.opts.AccountBlock)
}

// distinctSources counts the non-empty source addresses among the failures.
// Unknown sources (an empty string) never count, so a connection whose address
// cannot be determined can only ever block itself.
func distinctSources(failures []failure) map[string]struct{} {
	seen := make(map[string]struct{}, len(failures))
	for _, failed := range failures {
		if failed.source != "" {
			seen[failed.source] = struct{}{}
		}
	}
	return seen
}

func (l *Limiter) trackedLocked(key string, now time.Time) *entry {
	tracked, ok := l.entries[key]
	if !ok {
		l.makeRoomLocked(now)
		tracked = &entry{}
		l.entries[key] = tracked
	}
	tracked.lastSeen = now
	return tracked
}

// makeRoomLocked keeps the map bounded. Expired entries go first; if the map is
// still full the least recently seen key is dropped, which can only lose
// counters, never allow extra password hashing.
func (l *Limiter) makeRoomLocked(now time.Time) {
	if len(l.entries) < l.opts.MaxKeys {
		return
	}
	for key, tracked := range l.entries {
		idle := now.Sub(tracked.lastSeen) > l.opts.SourceWindow
		if idle && !now.Before(tracked.blockedUntil) {
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

func addressKey(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	return addressPrefix + strings.ToLower(source)
}

func accountKey(realm Realm, account string) string {
	account = strings.TrimSpace(account)
	if account == "" {
		return ""
	}
	return string(realm) + accountSuffix + strings.ToLower(account)
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
func Blocked(realm Realm, source, account string) bool {
	return config.LoginLimiterEnabled() && Default.Blocked(realm, source, account)
}

// BlockedSource is Blocked for the address alone.
func BlockedSource(source string) bool {
	return config.LoginLimiterEnabled() && Default.BlockedSource(source)
}

// Failure records a failed attempt on the shared limiter.
func Failure(realm Realm, source, account string) {
	if config.LoginLimiterEnabled() {
		Default.Failure(realm, source, account)
	}
}

// Success clears the shared limiter's counters for these keys.
func Success(realm Realm, source, account string) {
	Default.Success(realm, source, account)
}
