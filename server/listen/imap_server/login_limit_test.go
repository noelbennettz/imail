package imap_server

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/services/loginlimit"
	pcontext "github.com/noelbennettz/imail/utils/context"
	xormlog "xorm.io/xorm/log"
)

// limiterClock drives the failed sign-in limiter in tests.
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

func installLimiter(t *testing.T) *limiterClock {
	t.Helper()
	clock := &limiterClock{now: time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC)}
	opts := loginlimit.DefaultOptions
	opts.Now = clock.Now
	oldLimiter := loginlimit.Default
	oldEnabled := config.LoginLimiterEnabled()
	loginlimit.Default = loginlimit.New(opts)
	config.SetLoginLimiterEnabled(true)
	t.Cleanup(func() {
		loginlimit.Default = oldLimiter
		config.SetLoginLimiterEnabled(oldEnabled)
	})
	return clock
}

// sqlRecordingLogger records the statements xorm sends to the engine, so a test
// can prove that an authentication path never reached the database.
type sqlRecordingLogger struct {
	xormlog.DiscardLogger
	mu         sync.Mutex
	statements []string
}

func (l *sqlRecordingLogger) Debugf(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statements = append(l.statements, fmt.Sprintf(format, v...))
}

func (l *sqlRecordingLogger) IsShowSQL() bool { return true }

func (l *sqlRecordingLogger) ShowSQL(...bool) {}

func (l *sqlRecordingLogger) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statements = nil
}

func (l *sqlRecordingLogger) recorded() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.statements...)
}

func TestIMAPBlockedLoginSkipsDatabase(t *testing.T) {
	clock := installLimiter(t)
	recorder := &sqlRecordingLogger{}
	oldLogger := db.Instance.Logger()
	db.Instance.SetLogger(recorder)
	db.Instance.ShowSQL(true)
	t.Cleanup(func() { db.Instance.SetLogger(oldLogger) })

	session := &serverSession{
		server:     NewServer(),
		ctx:        &pcontext.Context{},
		remoteAddr: "192.0.2.40",
	}
	for i := 0; i < 5; i++ {
		if err := session.Login("testCase", "wrong-password"); err == nil {
			t.Fatalf("attempt %d with a wrong password succeeded", i+1)
		}
	}

	recorder.reset()
	err := session.Login("testCase", "testCase")
	if err == nil {
		t.Fatal("blocked login with the correct password succeeded")
	}
	var imapErr *imap.Error
	if !errors.As(err, &imapErr) {
		t.Fatalf("blocked login error = %T, want *imap.Error", err)
	}
	if imapErr.Code != imap.ResponseCodeAuthenticationFailed {
		t.Fatalf("blocked login code = %q, want AUTHENTICATIONFAILED", imapErr.Code)
	}
	if statements := recorder.recorded(); len(statements) != 0 {
		t.Fatalf("blocked login reached the database: %v", statements)
	}

	clock.Advance(15 * time.Minute)
	if err := session.Login("testCase", "testCase"); err != nil {
		t.Fatalf("login after the block window failed: %v", err)
	}
	if session.status != AUTHORIZED || session.ctx.UserID == 0 {
		t.Fatal("login after the block window did not authorise the session")
	}
}

func TestIMAPBlockedAccountFromAnotherAddress(t *testing.T) {
	installLimiter(t)
	first := &serverSession{server: NewServer(), ctx: &pcontext.Context{}, remoteAddr: "192.0.2.41"}
	for i := 0; i < 5; i++ {
		_ = first.Login("testCase", "wrong-password")
	}

	// A different address still cannot log in to the blocked account.
	second := &serverSession{server: NewServer(), ctx: &pcontext.Context{}, remoteAddr: "192.0.2.42"}
	err := second.Login("testCase", "testCase")
	if err == nil {
		t.Fatal("a blocked account accepted a login from another address")
	}
	var imapErr *imap.Error
	if !errors.As(err, &imapErr) || imapErr.Code != imap.ResponseCodeAuthenticationFailed {
		t.Fatalf("blocked login error = %v, want the ordinary authentication failure", err)
	}
}
