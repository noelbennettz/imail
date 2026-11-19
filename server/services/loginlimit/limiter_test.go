package loginlimit

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// fakeClock is a settable clock for the limiter.
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestLimiter(clock *fakeClock) *Limiter {
	opts := DefaultOptions
	opts.Now = clock.Now
	return New(opts)
}

func TestSourceBlockAfterFiveFailures(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.10", "alice@example.com"

	for i := 0; i < 4; i++ {
		if limiter.Blocked(RealmMail, source, account) {
			t.Fatalf("blocked after %d failures, want only after 5", i)
		}
		limiter.Failure(RealmMail, source, account)
	}
	if limiter.Blocked(RealmMail, source, account) {
		t.Fatal("blocked after 4 failures, want only after 5")
	}
	limiter.Failure(RealmMail, source, account)
	if !limiter.Blocked(RealmMail, source, account) {
		t.Fatal("not blocked after 5 failures")
	}

	// The block covers the last 14 minutes and expires with the 15-minute
	// block duration.
	clock.Advance(14*time.Minute + 59*time.Second)
	if !limiter.Blocked(RealmMail, source, account) {
		t.Fatal("block expired before 15 minutes")
	}
	clock.Advance(time.Second)
	if limiter.Blocked(RealmMail, source, account) {
		t.Fatal("still blocked after 15 minutes")
	}
}

func TestFailuresOutsideWindowDoNotAccumulate(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.11", "bob@example.com"

	for i := 0; i < 4; i++ {
		limiter.Failure(RealmMail, source, account)
		clock.Advance(5 * time.Minute)
	}
	// Four failures spread over 20 minutes; only the last one (at +15m) is
	// still inside the 15-minute window when the fifth arrives.
	limiter.Failure(RealmMail, source, account)
	if limiter.Blocked(RealmMail, source, account) {
		t.Fatal("blocked although the failures span more than the window")
	}
	for i := 0; i < 4; i++ {
		limiter.Failure(RealmMail, source, account)
	}
	if !limiter.Blocked(RealmMail, source, account) {
		t.Fatal("not blocked after five failures inside the window")
	}
}

func TestSuccessResetsCounters(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.12", "carol@example.com"

	for i := 0; i < 4; i++ {
		limiter.Failure(RealmMail, source, account)
	}
	limiter.Success(RealmMail, source, account)
	for i := 0; i < 4; i++ {
		limiter.Failure(RealmMail, source, account)
	}
	if limiter.Blocked(RealmMail, source, account) {
		t.Fatal("blocked even though a successful login reset the counters")
	}
	limiter.Failure(RealmMail, source, account)
	if !limiter.Blocked(RealmMail, source, account) {
		t.Fatal("not blocked after five fresh failures")
	}
}

func TestAddressAndAccountAreSeparateKeys(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)

	for i := 0; i < 5; i++ {
		limiter.Failure(RealmMail, "192.0.2.13", "dave@example.com")
	}
	if !limiter.Blocked(RealmMail, "192.0.2.13", "dave@example.com") {
		t.Fatal("pair not blocked")
	}
	// The same address from another account is blocked too.
	if !limiter.Blocked(RealmMail, "192.0.2.13", "erin@example.com") {
		t.Fatal("address key not blocked")
	}
	// The address key is shared with the web planes, so switching protocol
	// does not reset the block.
	if !limiter.Blocked(RealmWeb, "192.0.2.13", "dave@example.com") {
		t.Fatal("source key not shared across realms")
	}
	// An unrelated pair stays usable.
	if limiter.Blocked(RealmMail, "192.0.2.99", "erin@example.com") {
		t.Fatal("unrelated pair blocked")
	}
}

// TestOneSourceCannotLockAnAccount is the targeted-lockout test: a single
// attacker must never be able to lock a mailbox out, even when it fails the
// account many times (it is blocked itself after five).
func TestOneSourceCannotLockAnAccount(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	// Give the one source an unlimited source budget so the account rule is
	// what is under test.
	opts.SourceMaxFailures = 10000
	limiter := New(opts)
	const account = "target@example.com"

	for i := 0; i < 200; i++ {
		limiter.Failure(RealmMail, "198.51.100.7", account)
	}
	if limiter.Blocked(RealmMail, "192.0.2.200", account) {
		t.Fatal("a single source locked the account for everybody")
	}
}

func TestAccountBlocksOnlyFromManySources(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.AccountMaxFailures = 6
	opts.AccountMinSources = 3
	opts.SourceMaxFailures = 10000
	limiter := New(opts)
	const account = "shared@example.com"

	// Two sources with three failures each: six failures but only two
	// distinct sources, so the account stays usable.
	for i := 0; i < 3; i++ {
		limiter.Failure(RealmMail, "198.51.100.1", account)
		limiter.Failure(RealmMail, "198.51.100.2", account)
	}
	if limiter.Blocked(RealmMail, "192.0.2.201", account) {
		t.Fatal("account blocked although only two sources contributed")
	}

	// A third source reaches the threshold.
	for i := 0; i < 3; i++ {
		limiter.Failure(RealmMail, "198.51.100.3", account)
	}
	if !limiter.Blocked(RealmMail, "192.0.2.201", account) {
		t.Fatal("account not blocked after six failures from three sources")
	}
	// Other accounts are unaffected.
	if limiter.Blocked(RealmMail, "192.0.2.201", "other@example.com") {
		t.Fatal("unrelated account blocked")
	}
	clock.Advance(opts.AccountBlock)
	if limiter.Blocked(RealmMail, "192.0.2.201", account) {
		t.Fatal("account block did not expire")
	}
}

// TestDistributedSprayDoesNotBlockCorrectLogin is the denial-of-service
// regression test: a hundred failures from a hundred sources must not stop a
// correct login that has failures of its own.
func TestDistributedSprayDoesNotBlockCorrectLogin(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	alerts := 0
	opts.OnAlert = func(int, time.Duration) { alerts++ }
	limiter := New(opts)

	for i := 0; i < 100; i++ {
		source := "198.51.100." + strconv.Itoa(i)
		limiter.Failure(RealmWeb, source, "user"+strconv.Itoa(i)+"@example.com")
	}
	if got := limiter.GlobalFailures(); got < 100 {
		t.Fatalf("global counter = %d, want at least 100", got)
	}
	if alerts == 0 {
		t.Fatal("the global counter did not alert")
	}

	// The legitimate user failed once before, and the spray is still going on.
	limiter.Failure(RealmWeb, "203.0.113.5", "admin@example.com")
	if limiter.Blocked(RealmWeb, "203.0.113.5", "admin@example.com") {
		t.Fatal("a correct login is blocked by the global failure counter")
	}
	limiter.Failure(RealmWeb, "203.0.113.5", "admin@example.com")
	if limiter.Blocked(RealmWeb, "203.0.113.5", "admin@example.com") {
		t.Fatal("a correct login is blocked while a distributed spray runs")
	}
	limiter.Success(RealmWeb, "203.0.113.5", "admin@example.com")
	if limiter.Blocked(RealmWeb, "203.0.113.5", "admin@example.com") {
		t.Fatal("successful login left the keys blocked")
	}
}

func TestRealmsKeepAccountCountersSeparate(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.AccountMaxFailures = 6
	opts.AccountMinSources = 3
	opts.SourceMaxFailures = 10000
	limiter := New(opts)

	for i := 0; i < 3; i++ {
		limiter.Failure(RealmWeb, "198.51.100.1", "admin")
		limiter.Failure(RealmWeb, "198.51.100.2", "admin")
		limiter.Failure(RealmWeb, "198.51.100.3", "admin")
	}
	if !limiter.Blocked(RealmWeb, "203.0.113.9", "admin") {
		t.Fatal("webmail account key not blocked")
	}
	// The administrator console has its own counter for the same name.
	if limiter.Blocked(RealmAdmin, "203.0.113.9", "admin") {
		t.Fatal("a webmail spray blocked the admin console account key")
	}
	if limiter.Blocked(RealmMail, "203.0.113.9", "admin") {
		t.Fatal("a webmail spray blocked the mail account key")
	}
}

func TestMapIsBounded(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.MaxKeys = 8
	limiter := New(opts)

	for i := 0; i < 100; i++ {
		limiter.Failure(RealmMail, "198.51.100."+strconv.Itoa(i), "user"+strconv.Itoa(i)+"@example.com")
		clock.Advance(time.Second)
		if got := limiter.Keys(); got > opts.MaxKeys {
			t.Fatalf("tracked keys = %d, want at most %d", got, opts.MaxKeys)
		}
	}
	if got := limiter.Keys(); got == 0 {
		t.Fatal("limiter forgot every key")
	}
}

func TestEmptyKeysAreIgnored(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)

	for i := 0; i < 10; i++ {
		limiter.Failure(RealmMail, "", "")
	}
	if limiter.Keys() != 0 {
		t.Fatalf("tracked keys = %d, want 0", limiter.Keys())
	}
	if limiter.Blocked(RealmMail, "", "") {
		t.Fatal("empty keys must never be blocked")
	}
}

func TestUnknownSourceNeverBlocksAnAccount(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.AccountMaxFailures = 4
	opts.SourceMaxFailures = 10000
	limiter := New(opts)

	for i := 0; i < 50; i++ {
		limiter.Failure(RealmWeb, "", "someone@example.com")
	}
	if limiter.Blocked(RealmWeb, "203.0.113.7", "someone@example.com") {
		t.Fatal("failures without a source address blocked an account")
	}
}

func TestSourceKey(t *testing.T) {
	cases := []struct {
		name string
		addr net.Addr
		want string
	}{
		{name: "ipv4", addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.50"), Port: 1234}, want: "192.0.2.50"},
		{name: "ipv6 with zone", addr: &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 25, Zone: "eth0"}, want: "2001:db8::1"},
		{name: "nil", addr: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SourceKey(tc.addr); got != tc.want {
				t.Fatalf("SourceKey() = %q, want %q", got, tc.want)
			}
		})
	}
}
