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

func TestBlockAfterFiveFailures(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.10", "alice@example.com"

	for i := 0; i < 4; i++ {
		if limiter.Blocked(source, account) {
			t.Fatalf("blocked after %d failures, want only after 5", i)
		}
		limiter.Failure(source, account)
	}
	if limiter.Blocked(source, account) {
		t.Fatal("blocked after 4 failures, want only after 5")
	}
	limiter.Failure(source, account)
	if !limiter.Blocked(source, account) {
		t.Fatal("not blocked after 5 failures")
	}

	// The block covers the last 14 minutes and expires with the 15-minute
	// block duration.
	clock.Advance(14*time.Minute + 59*time.Second)
	if !limiter.Blocked(source, account) {
		t.Fatal("block expired before 15 minutes")
	}
	clock.Advance(time.Second)
	if limiter.Blocked(source, account) {
		t.Fatal("still blocked after 15 minutes")
	}
}

func TestFailuresOutsideWindowDoNotAccumulate(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.11", "bob@example.com"

	for i := 0; i < 4; i++ {
		limiter.Failure(source, account)
		clock.Advance(5 * time.Minute)
	}
	// Four failures spread over 20 minutes; only the last one (at +15m) is
	// still inside the 15-minute window when the fifth arrives.
	limiter.Failure(source, account)
	if limiter.Blocked(source, account) {
		t.Fatal("blocked although the failures span more than the window")
	}
	for i := 0; i < 4; i++ {
		limiter.Failure(source, account)
	}
	if !limiter.Blocked(source, account) {
		t.Fatal("not blocked after five failures inside the window")
	}
}

func TestSuccessResetsCounters(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)
	const source, account = "192.0.2.12", "carol@example.com"

	for i := 0; i < 4; i++ {
		limiter.Failure(source, account)
	}
	limiter.Success(source, account)
	for i := 0; i < 4; i++ {
		limiter.Failure(source, account)
	}
	if limiter.Blocked(source, account) {
		t.Fatal("blocked even though a successful login reset the counters")
	}
	limiter.Failure(source, account)
	if !limiter.Blocked(source, account) {
		t.Fatal("not blocked after five fresh failures")
	}
}

func TestAddressAndAccountAreSeparateKeys(t *testing.T) {
	clock := newFakeClock()
	limiter := newTestLimiter(clock)

	for i := 0; i < 5; i++ {
		limiter.Failure("192.0.2.13", "dave@example.com")
	}
	if !limiter.Blocked("192.0.2.13", "dave@example.com") {
		t.Fatal("pair not blocked")
	}
	// The same address from another account is blocked too.
	if !limiter.Blocked("192.0.2.13", "erin@example.com") {
		t.Fatal("address key not blocked")
	}
	// Another address cannot use the blocked account.
	if !limiter.Blocked("192.0.2.99", "dave@example.com") {
		t.Fatal("account key not blocked")
	}
	// An unrelated pair stays usable.
	if limiter.Blocked("192.0.2.99", "erin@example.com") {
		t.Fatal("unrelated pair blocked")
	}
}

func TestGlobalCeilingStopsDistributedSpray(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.GlobalMaxFailures = 10
	limiter := New(opts)

	// Ten different addresses and accounts, one failure each: no key reaches
	// the per-key limit, but the global window fills up.
	for i := 0; i < opts.GlobalMaxFailures; i++ {
		source := "192.0.2." + strconv.Itoa(100+i)
		account := "user" + strconv.Itoa(i) + "@example.com"
		if limiter.Blocked(source, account) {
			t.Fatalf("attempt %d refused before the ceiling", i)
		}
		limiter.Failure(source, account)
	}
	if !limiter.Blocked("198.51.100.1", "fresh@example.com") {
		t.Fatal("fresh keys not refused after the global ceiling")
	}
	clock.Advance(opts.GlobalWindow)
	if limiter.Blocked("198.51.100.1", "fresh@example.com") {
		t.Fatal("global ceiling did not clear after its window")
	}
}

func TestMapIsBounded(t *testing.T) {
	clock := newFakeClock()
	opts := DefaultOptions
	opts.Now = clock.Now
	opts.MaxKeys = 8
	opts.GlobalMaxFailures = 1000
	limiter := New(opts)

	for i := 0; i < 100; i++ {
		limiter.Failure("192.0.2."+strconv.Itoa(i), "user"+strconv.Itoa(i)+"@example.com")
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
		limiter.Failure("", "")
	}
	if limiter.Keys() != 0 {
		t.Fatalf("tracked keys = %d, want 0", limiter.Keys())
	}
	if limiter.Blocked("", "") {
		t.Fatal("empty keys must never be blocked")
	}
}

func TestSourceKey(t *testing.T) {
	cases := []struct {
		name string
		addr net.Addr
		want string
	}{
		{name: "ipv4", addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.50"), Port: 1234}, want: "192.0.2.50"},
		{name: "ipv4 with mapped zone", addr: &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 25, Zone: "eth0"}, want: "2001:db8::1"},
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
