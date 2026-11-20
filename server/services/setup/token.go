package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// The setup wizard runs before TLS is configured, so anyone who can reach the
// HTTP port could have completed the initialization first. A one-time token,
// generated at every start of an uninitialised deployment and printed only to
// the logs, closes that window. The token lives in memory only: it is never
// written to config/ or to disk.
const (
	// TokenHeader carries the setup token on every setup API request.
	TokenHeader = "X-Setup-Token"
	// tokenLength is the number of characters of a generated token.
	tokenLength = 24
	// tokenAlphabet keeps the token copyable from a log line.
	tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	// maxFailuresPerAddress blocks one source address after this many failures.
	maxFailuresPerAddress = 5
	// failureWindow is both the length of the counting window and the length of
	// a block applied after too many failures.
	failureWindow = 15 * time.Minute
)

// VerifyResult is the outcome of checking a presented setup token.
type VerifyResult int

const (
	// VerifyAllowed means the token is correct and the setup API may run.
	VerifyAllowed VerifyResult = iota
	// VerifyRejected means the token was missing or wrong.
	VerifyRejected
	// VerifyRateLimited means too many failures; the caller gets 429.
	VerifyRateLimited
	// VerifyClosed means the setup phase is over; the API is closed for good.
	VerifyClosed
)

// HTTPStatus maps a result to the HTTP status code the handler must return.
func (r VerifyResult) HTTPStatus() int {
	switch r {
	case VerifyAllowed:
		return http.StatusOK
	case VerifyRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusForbidden
	}
}

type addressFailures struct {
	windowStart  time.Time
	count        int
	blockedUntil time.Time
}

// SetupToken holds the one-time token of the current process together with the
// brute-force counters. It is safe for concurrent use.
type SetupToken struct {
	mu        sync.Mutex
	token     string
	enabled   bool
	now       func() time.Time
	addresses map[string]*addressFailures
}

// NewSetupToken generates the one-time setup token. On an initialised
// deployment nothing is generated and nothing is logged. Otherwise the token is
// printed once in a banner that says how to read it again.
func NewSetupToken(initialized bool) (*SetupToken, error) {
	t := &SetupToken{
		enabled:   !initialized,
		now:       time.Now,
		addresses: map[string]*addressFailures{},
	}
	if initialized {
		return t, nil
	}

	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	t.token = token
	logBanner(token)
	return t, nil
}

func generateToken() (string, error) {
	// 62 does not divide 256: reject bytes from the incomplete tail so every
	// character stays equally likely.
	const tail = 256 - (256 % len(tokenAlphabet))

	out := make([]byte, 0, tokenLength)
	buf := make([]byte, tokenLength*2)
	for len(out) < tokenLength {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= tail {
				continue
			}
			out = append(out, tokenAlphabet[int(b)%len(tokenAlphabet)])
			if len(out) == tokenLength {
				break
			}
		}
	}
	return string(out), nil
}

func logBanner(token string) {
	log.Warnln("*******************************************************************")
	log.Warnf("***  Setup token: %s", token)
	log.Warnln("***")
	log.Warnln("***  The setup wizard asks for this token on its first screen and")
	log.Warnln("***  sends it as the " + TokenHeader + " header on every setup request.")
	log.Warnln("***  It is kept in memory only and disappears once setup is finished.")
	log.Warnln("***  Read it again with: docker compose logs imail")
	log.Warnln("*******************************************************************")
}

// Enabled reports whether the setup API still accepts requests.
func (t *SetupToken) Enabled() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enabled
}

// Token returns the current token, or "" when there is none. It exists for the
// setup handlers and for tests that drive the HTTP API directly.
func (t *SetupToken) Token() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.token
}

// Clear forgets the token and closes the setup API permanently. It runs once
// initialization has completed successfully.
func (t *SetupToken) Clear() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = ""
	t.enabled = false
	t.addresses = map[string]*addressFailures{}
}

// Verify checks a presented token against the current one. Failures are
// counted per source address; the token comparison itself takes constant time.
func (t *SetupToken) Verify(remoteAddr, presented string) VerifyResult {
	if t == nil {
		return VerifyClosed
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return VerifyClosed
	}

	now := t.now()
	address := remoteAddress(remoteAddr)
	t.pruneExpired(now)

	if t.blocked(address, now) {
		return VerifyRateLimited
	}

	if !tokenEquals(t.token, presented) {
		t.recordFailure(address, now)
		log.Warnf("Setup token rejected for %s", address)
		return VerifyRejected
	}
	return VerifyAllowed
}

func (t *SetupToken) blocked(address string, now time.Time) bool {
	entry, ok := t.addresses[address]
	if !ok {
		return false
	}
	return entry.blockedUntil.After(now)
}

// recordFailure counts one failed attempt for an address. The per-address
// counter only ever expires, so a flood cannot keep a legitimate administrator
// locked out forever and cannot shut the setup API down for everyone.
func (t *SetupToken) recordFailure(address string, now time.Time) {
	entry, ok := t.addresses[address]
	if !ok || now.Sub(entry.windowStart) >= failureWindow {
		entry = &addressFailures{windowStart: now}
		t.addresses[address] = entry
	}
	entry.count++
	if entry.count >= maxFailuresPerAddress {
		entry.blockedUntil = now.Add(failureWindow)
		log.Warnf("Setup token: too many failures from %s, blocked for %s", address, failureWindow)
	}
}

func (t *SetupToken) pruneExpired(now time.Time) {
	for address, entry := range t.addresses {
		if !entry.blockedUntil.After(now) && now.Sub(entry.windowStart) >= failureWindow {
			delete(t.addresses, address)
		}
	}
}

// tokenEquals compares two tokens without leaking where they differ. The
// presented value is padded to the expected length so the comparison always
// touches the same number of bytes.
func tokenEquals(expected, presented string) bool {
	if expected == "" {
		return false
	}
	padded := make([]byte, len(expected))
	copy(padded, presented)
	equal := subtle.ConstantTimeCompare([]byte(expected), padded) == 1
	if len(presented) != len(expected) {
		return false
	}
	return equal
}

func remoteAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

// currentValue is the token of the running process. It is nil outside the setup
// phase and after a successful initialization.
var (
	currentMu    sync.Mutex
	currentValue *SetupToken
)

// InitToken creates the process-wide setup token. Pass whether the deployment
// is already initialised: an initialised deployment gets no token and no log
// output at all.
func InitToken(initialized bool) (*SetupToken, error) {
	token, err := NewSetupToken(initialized)
	if err != nil {
		return nil, err
	}
	currentMu.Lock()
	currentValue = token
	currentMu.Unlock()
	return token, nil
}

// CurrentToken returns the process-wide setup token, or nil when the process
// has none.
func CurrentToken() *SetupToken {
	currentMu.Lock()
	defer currentMu.Unlock()
	return currentValue
}

// ClearToken forgets the process-wide setup token and closes the setup API.
func ClearToken() {
	currentMu.Lock()
	token := currentValue
	currentValue = nil
	currentMu.Unlock()
	token.Clear()
}
