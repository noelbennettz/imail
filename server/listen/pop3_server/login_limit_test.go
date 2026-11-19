package pop3_server

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/third_party/gopop"
	pcontext "github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/password"
	"xorm.io/xorm"
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

// stubConn provides a client address for a session that is not on the wire.
type stubConn struct {
	remote net.Addr
}

func (c stubConn) Read([]byte) (int, error)         { return 0, errors.New("not a real connection") }
func (c stubConn) Write([]byte) (int, error)        { return 0, errors.New("not a real connection") }
func (c stubConn) Close() error                     { return nil }
func (c stubConn) LocalAddr() net.Addr              { return c.remote }
func (c stubConn) RemoteAddr() net.Addr             { return c.remote }
func (c stubConn) SetDeadline(time.Time) error      { return nil }
func (c stubConn) SetReadDeadline(time.Time) error  { return nil }
func (c stubConn) SetWriteDeadline(time.Time) error { return nil }

func newPOP3TestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", filepath.Join(t.TempDir(), "pop3.db"))
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	engine.SetMaxIdleConns(1)
	if err = engine.Sync2(&models.User{}); err != nil {
		_ = engine.Close()
		t.Fatal(err)
	}
	oldDB := db.Instance
	oldConfig := config.Instance
	db.Instance = engine
	config.Instance = &config.Config{DbType: config.DBTypeSQLite, Domain: "example.com", Domains: []string{"example.com"}}
	t.Cleanup(func() {
		db.Instance = oldDB
		config.Instance = oldConfig
		_ = engine.Close()
	})
	return engine
}

func TestPOP3BlockedPassSkipsDatabase(t *testing.T) {
	engine := newPOP3TestEngine(t)
	clock := installLimiter(t)
	if _, err := engine.Insert(&models.User{
		Account:  "pop-user",
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

	newSession := func() *gopop.Session {
		return &gopop.Session{
			Status: gopop.UNAUTHORIZED,
			User:   "pop-user",
			InTls:  true,
			Ctx:    &pcontext.Context{},
			Conn:   stubConn{remote: &net.TCPAddr{IP: net.ParseIP("192.0.2.50"), Port: 110}},
		}
	}
	a := action{}
	for i := 0; i < 5; i++ {
		if err := a.Pass(newSession(), "wrong-password"); err == nil {
			t.Fatalf("attempt %d with a wrong password succeeded", i+1)
		}
	}

	blocked := newSession()
	recorder.reset()
	err := a.Pass(blocked, "correct-horse")
	if err == nil {
		t.Fatal("blocked PASS with the correct password succeeded")
	}
	if err.Error() != "password error" {
		t.Fatalf("blocked PASS error = %q, want the ordinary authentication failure", err.Error())
	}
	if blocked.Status != gopop.UNAUTHORIZED {
		t.Fatal("blocked PASS moved the session out of the authorisation state")
	}
	if statements := recorder.recorded(); len(statements) != 0 {
		t.Fatalf("blocked PASS reached the database: %v", statements)
	}

	clock.Advance(15 * time.Minute)
	allowed := newSession()
	if err := a.Pass(allowed, "correct-horse"); err != nil {
		t.Fatalf("PASS after the block window failed: %v", err)
	}
	if allowed.Status != gopop.TRANSACTION {
		t.Fatal("PASS after the block window did not authorise the session")
	}
}

func TestPOP3BlockedSourceCannotTryAnotherAccount(t *testing.T) {
	engine := newPOP3TestEngine(t)
	installLimiter(t)
	if _, err := engine.Insert(&models.User{
		Account:  "other-user",
		Password: password.Encode("correct-horse"),
		Domain:   "example.com",
	}); err != nil {
		t.Fatal(err)
	}
	conn := stubConn{remote: &net.TCPAddr{IP: net.ParseIP("192.0.2.51"), Port: 110}}
	a := action{}
	for i := 0; i < 5; i++ {
		session := &gopop.Session{User: "unknown-user", InTls: true, Ctx: &pcontext.Context{}, Conn: conn}
		_ = a.Pass(session, "wrong-password")
	}
	session := &gopop.Session{User: "other-user", InTls: true, Ctx: &pcontext.Context{}, Conn: conn}
	if err := a.Pass(session, "correct-horse"); err == nil {
		t.Fatal("a blocked address authenticated as another account")
	}
}
