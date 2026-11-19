package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/loginlimit"
)

// limiterClock is a settable clock for the web-login limiter tests.
type limiterClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *limiterClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *limiterClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// installLimiter replaces the shared limiter with one that uses a fake clock.
func installLimiter(t *testing.T, tweak func(*loginlimit.Options)) *limiterClock {
	t.Helper()
	clock := &limiterClock{now: time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC)}
	opts := loginlimit.DefaultOptions
	opts.Now = clock.Now
	if tweak != nil {
		tweak(&opts)
	}
	old := loginlimit.Default
	loginlimit.Default = loginlimit.New(opts)
	t.Cleanup(func() { loginlimit.Default = old })
	return clock
}

// callFrom posts /api/login as if it came from source. The test clients connect
// from 127.0.0.1, so a forwarded address is what the limiter keys on.
func (f *authFixture) callFrom(t *testing.T, source, path string, body any) response.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", source)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var result response.Response
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWebLoginLimiterBlocksTheAddress(t *testing.T) {
	f := newAuthFixture(t)
	clock := installLimiter(t, nil)
	const source = "203.0.113.20"

	for i := 0; i < 5; i++ {
		result := f.callFrom(t, source, "/api/login", map[string]any{"account": "member", "password": "wrong"})
		if result.ErrorNo == 0 {
			t.Fatalf("attempt %d with a wrong password succeeded", i+1)
		}
	}
	blocked := f.callFrom(t, source, "/api/login", map[string]any{"account": "member", "password": "initial-password"})
	if blocked.ErrorNo == 0 {
		t.Fatal("the blocked address logged in with the correct password")
	}
	// Another address is unaffected.
	if result := f.callFrom(t, "203.0.113.21", "/api/login", map[string]any{"account": "member", "password": "initial-password"}); result.ErrorNo != 0 {
		t.Fatalf("an unrelated address was blocked: %+v", result)
	}
	// The block expires after fifteen minutes.
	clock.Advance(15 * time.Minute)
	if result := f.callFrom(t, source, "/api/login", map[string]any{"account": "member", "password": "initial-password"}); result.ErrorNo != 0 {
		t.Fatalf("login after the block window failed: %+v", result)
	}
}

func TestWebLoginLimiterKeepsAdminRealmSeparate(t *testing.T) {
	f := newAuthFixture(t)
	installLimiter(t, func(opts *loginlimit.Options) {
		opts.SourceMaxFailures = 10000
		opts.AccountMaxFailures = 6
		opts.AccountMinSources = 3
	})

	// A distributed spray at a user mailbox. The account key is namespaced by
	// the authenticated role, so these failures land in the webmail realm.
	for i := 0; i < 3; i++ {
		source := "198.51.100." + strconv.Itoa(10+i)
		for j := 0; j < 2; j++ {
			f.callFrom(t, source, "/api/login", map[string]any{"account": "member", "password": "wrong"})
		}
	}

	// The mailbox is blocked for everybody, including the correct password
	// from an address that has no failures of its own.
	if result := f.callFrom(t, "203.0.113.30", "/api/login", map[string]any{"account": "member", "password": "initial-password"}); result.ErrorNo == 0 {
		t.Fatal("the webmail account key was not blocked")
	}
	// The administrator account has its own counter and is not affected.
	if result := f.callFrom(t, "203.0.113.31", "/api/login", map[string]any{"account": "admin", "password": "initial-password"}); result.ErrorNo != 0 {
		t.Fatalf("the administrator login was blocked by a user-account spray: %+v", result)
	}
}

func TestWebLoginSurvivesADistributedSpray(t *testing.T) {
	f := newAuthFixture(t)
	installLimiter(t, nil)

	for i := 0; i < 100; i++ {
		source := "198.51.100." + strconv.Itoa(100+i)
		result := f.callFrom(t, source, "/api/login", map[string]any{
			"account":  "user" + strconv.Itoa(i),
			"password": "wrong",
		})
		if result.ErrorNo == 0 {
			t.Fatal("a login with a wrong password succeeded")
		}
	}

	// A legitimate login still works: the distributed failures only feed the
	// alert counter, they never refuse an attempt.
	result := f.callFrom(t, "203.0.113.40", "/api/login", map[string]any{
		"account":  "member",
		"password": "initial-password",
	})
	if result.ErrorNo != 0 {
		t.Fatalf("a correct login was refused after a distributed spray: %+v", result)
	}
}
