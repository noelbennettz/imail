package smtp_server

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/password"
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

// installLimiter replaces the shared limiter with one that uses a fake clock
// and switches the feature on for the duration of the test.
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

func TestSMTPBlockedAuthenticationSkipsDatabase(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	clock := installLimiter(t)
	if _, err := engine.Insert(&models.User{
		Account:  "limited-user",
		Password: password.Encode("correct-horse"),
		Domain:   "example.com",
	}); err != nil {
		t.Fatal(err)
	}
	recorder := &sqlRecordingLogger{}
	oldLogger := engine.Logger()
	engine.SetLogger(recorder)
	engine.ShowSQL(true)
	t.Cleanup(func() { engine.SetLogger(oldLogger) })

	session := &Session{
		RemoteAddress: &net.TCPAddr{IP: net.ParseIP("192.0.2.30"), Port: 2525},
		Ctx:           &context.Context{},
	}
	for i := 0; i < 5; i++ {
		if err := session.AuthPlain("limited-user", "wrong-password"); err == nil {
			t.Fatalf("attempt %d with a wrong password succeeded", i+1)
		}
	}

	recorder.reset()
	err := session.AuthPlain("limited-user", "correct-horse")
	if err == nil {
		t.Fatal("blocked attempt with the correct password succeeded")
	}
	if err.Error() != "password error" {
		t.Fatalf("blocked attempt error = %q, want the ordinary authentication failure", err.Error())
	}
	if statements := recorder.recorded(); len(statements) != 0 {
		t.Fatalf("blocked attempt reached the database: %v", statements)
	}

	clock.Advance(15 * time.Minute)
	if err := session.AuthPlain("limited-user", "correct-horse"); err != nil {
		t.Fatalf("login after the block window failed: %v", err)
	}
	if session.Ctx.UserID == 0 {
		t.Fatal("login after the block window did not authenticate the session")
	}
}

func TestSMTPBlockedSourceCannotTryAnotherAccount(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	installLimiter(t)
	if _, err := engine.Insert(&models.User{
		Account:  "second-user",
		Password: password.Encode("correct-horse"),
		Domain:   "example.com",
	}); err != nil {
		t.Fatal(err)
	}

	session := &Session{
		RemoteAddress: &net.TCPAddr{IP: net.ParseIP("192.0.2.31"), Port: 2525},
		Ctx:           &context.Context{},
	}
	for i := 0; i < 5; i++ {
		_ = session.AuthPlain("someone-else", "wrong-password")
	}
	err := session.AuthPlain("second-user", "correct-horse")
	if err == nil {
		t.Fatal("a blocked address authenticated as another account")
	}
	if err.Error() != "password error" {
		t.Fatalf("blocked attempt error = %q, want the ordinary authentication failure", err.Error())
	}
}

func TestSMTPLimiterCanBeSwitchedOff(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	installLimiter(t)
	config.SetLoginLimiterEnabled(false)
	if _, err := engine.Insert(&models.User{
		Account:  "unlimited-user",
		Password: password.Encode("correct-horse"),
		Domain:   "example.com",
	}); err != nil {
		t.Fatal(err)
	}

	session := &Session{
		RemoteAddress: &net.TCPAddr{IP: net.ParseIP("192.0.2.32"), Port: 2525},
		Ctx:           &context.Context{},
	}
	for i := 0; i < 10; i++ {
		_ = session.AuthPlain("unlimited-user", "wrong-password")
	}
	if err := session.AuthPlain("unlimited-user", "correct-horse"); err != nil {
		t.Fatalf("login with the switch off failed: %v", err)
	}
}

// TestSMTPCorrectLoginSurvivesADistributedSpray is the denial-of-service
// regression test: a hundred failures from a hundred sources must not stop the
// legitimate owner of a mailbox from logging in.
func TestSMTPCorrectLoginSurvivesADistributedSpray(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	installLimiter(t)
	if _, err := engine.Insert(&models.User{
		Account:  "spray-target",
		Password: password.Encode("correct-horse"),
		Domain:   "example.com",
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		session := &Session{
			RemoteAddress: &net.TCPAddr{IP: net.ParseIP(fmt.Sprintf("198.51.100.%d", i+1)), Port: 2500},
			Ctx:           &context.Context{},
		}
		account := fmt.Sprintf("user%d", i)
		if err := session.AuthPlain(account, "wrong-password"); err == nil {
			t.Fatalf("spray attempt %d succeeded", i)
		}
	}

	session := &Session{
		RemoteAddress: &net.TCPAddr{IP: net.ParseIP("203.0.113.60"), Port: 2525},
		Ctx:           &context.Context{},
	}
	if err := session.AuthPlain("spray-target", "correct-horse"); err != nil {
		t.Fatalf("correct login refused after a distributed spray: %v", err)
	}
	if session.Ctx.UserAccount != "spray-target" {
		t.Fatal("the session was not authenticated")
	}
}
