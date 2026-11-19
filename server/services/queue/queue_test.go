package queue

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"xorm.io/xorm"
)

// newTestEngine returns a fresh SQLite database with the queue schema and points
// the package at it.
func newTestEngine(t *testing.T) (*xorm.Engine, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "queue.db")
	engine := openTestEngine(t, dsn)
	return engine, dsn
}

func openTestEngine(t *testing.T, dsn string) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	engine.SetMaxIdleConns(1)
	if err = engine.Sync2(&models.User{}, &models.Email{}, &models.UserEmail{}, &models.EmailQueue{}); err != nil {
		_ = engine.Close()
		t.Fatal(err)
	}
	// The server pins time.Local to UTC (config.Init); the SQLite driver stores
	// timestamps in the local zone, so the tests do the same to round-trip them.
	oldLocal := time.Local
	time.Local = time.UTC
	oldDB := db.Instance
	oldConfig := config.Instance
	db.Instance = engine
	config.Instance = &config.Config{DbType: config.DBTypeSQLite, Domain: "example.com", Domains: []string{"example.com"}}
	t.Cleanup(func() {
		db.Instance = oldDB
		config.Instance = oldConfig
		_ = engine.Close()
		time.Local = oldLocal
	})
	return engine
}

func insertEmail(t *testing.T, engine *xorm.Engine, messageID string) models.Email {
	t.Helper()
	email := models.Email{
		Type:        1,
		Subject:     "queued",
		FromAddress: "sender@example.com",
		RawMIME:     "From: sender@example.com\r\nTo: recipient@example.net\r\nSubject: queued\r\n\r\nbody\r\n",
		Status:      6,
		MsgID:       messageID,
		SendDate:    time.Now(),
		CreateTime:  time.Now(),
	}
	if _, err := engine.Insert(&email); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.UserEmail{UserID: 1, EmailID: email.Id, Status: 6}); err != nil {
		t.Fatal(err)
	}
	return email
}

func enqueue(t *testing.T, engine *xorm.Engine, email models.Email, recipients ...string) models.EmailQueue {
	t.Helper()
	return enqueueAt(t, engine, email, time.Now(), recipients...)
}

// enqueueAt writes an entry as if it had been accepted at now. The timestamps
// are forced after the insert because xorm fills the create_time tag itself.
func enqueueAt(t *testing.T, engine *xorm.Engine, email models.Email, now time.Time, recipients ...string) models.EmailQueue {
	t.Helper()
	entry, err := Entry(email.Id, email.MsgID, email.FromAddress, recipients, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&entry); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Exec("update email_queue set create_time=?, next_attempt=?, update_time=? where id=?",
		now, now, now, entry.Id); err != nil {
		t.Fatal(err)
	}
	entry.CreateTime = now
	entry.NextAttempt = now
	entry.UpdateTime = now
	return entry
}

func loadEntry(t *testing.T, engine *xorm.Engine, id int64) models.EmailQueue {
	t.Helper()
	var entry models.EmailQueue
	found, err := engine.ID(id).Get(&entry)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("queue entry %d not found", id)
	}
	return entry
}

func loadEmail(t *testing.T, engine *xorm.Engine, id int) models.Email {
	t.Helper()
	var email models.Email
	found, err := engine.ID(id).Get(&email)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("email %d not found", id)
	}
	return email
}

func TestRetrySchedule(t *testing.T) {
	want := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
		6 * time.Hour,
		24 * time.Hour,
		24 * time.Hour,
	}
	for i, expected := range want {
		if got := RetryDelay(i + 1); got != expected {
			t.Fatalf("RetryDelay(%d) = %s, want %s", i+1, got, expected)
		}
	}
	if got := RetryDelay(0); got != time.Minute {
		t.Fatalf("RetryDelay(0) = %s, want the first delay", got)
	}
}

func TestNextAttemptStopsAtMaxRetryAge(t *testing.T) {
	created := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	next, keepGoing := NextAttempt(created, created, 1)
	if !keepGoing || !next.Equal(created.Add(time.Minute)) {
		t.Fatalf("first retry = %s keepGoing=%t, want +1m", next, keepGoing)
	}

	// A temporary failure three days later cannot be retried any more.
	late := created.Add(MaxRetryAge - time.Second)
	if _, keepGoing = NextAttempt(late, created, 6); keepGoing {
		t.Fatal("retry scheduled beyond the maximum age")
	}

	// A short delay just inside the window is still scheduled.
	inside := created.Add(MaxRetryAge - 2*time.Minute)
	next, keepGoing = NextAttempt(inside, created, 1)
	if !keepGoing || next.After(created.Add(MaxRetryAge)) {
		t.Fatalf("retry inside the window = %s keepGoing=%t", next, keepGoing)
	}
}

func TestSplitRecipients(t *testing.T) {
	local, remote := SplitRecipients(
		[]string{"alice@example.com", "bob@Example.COM", "carol@other.example", "carol@other.example", ""},
		[]string{"example.com"},
	)
	if len(local) != 2 || local[0] != "alice@example.com" || local[1] != "bob@Example.COM" {
		t.Fatalf("local = %v, want the two local addresses", local)
	}
	if len(remote) != 1 || remote[0] != "carol@other.example" {
		t.Fatalf("remote = %v, want the single remote address", remote)
	}
}

func TestClaimLeasesOnce(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "claim-once@example.com")
	entry := enqueue(t, engine, email, "recipient@example.net")

	now := time.Now()
	claimed, err := Claim(now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Id != entry.Id {
		t.Fatalf("Claim() = %+v, want entry %d", claimed, entry.Id)
	}
	if claimed.Attempts != 1 || claimed.State != models.EmailQueueDelivering {
		t.Fatalf("claimed entry = state %d attempts %d, want delivering/1", claimed.State, claimed.Attempts)
	}
	if again, err := Claim(now, time.Minute); err != nil || again != nil {
		t.Fatalf("second Claim() = %+v, %v; want nothing while the lease is live", again, err)
	}

	// Once the lease expires the row is claimed again: this is how a delivery
	// interrupted by a crash gets retried.
	reclaimed, err := Claim(now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed == nil || reclaimed.Id != entry.Id {
		t.Fatalf("reclaim after lease expiry = %+v, want entry %d", reclaimed, entry.Id)
	}
	if reclaimed.Attempts != 2 {
		t.Fatalf("attempts after reclaim = %d, want 2", reclaimed.Attempts)
	}
}

func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "concurrent-claim@example.com")
	entry := enqueue(t, engine, email, "recipient@example.net")
	now := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := Claim(now, time.Minute)
			if err != nil {
				t.Errorf("Claim(): %v", err)
				return
			}
			if claimed != nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("concurrent claims = %d winners, want exactly 1", winners)
	}
	if got := loadEntry(t, engine, entry.Id); got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
}

func TestRestartRecoveryKeepsPendingEntries(t *testing.T) {
	engine, dsn := newTestEngine(t)
	email := insertEmail(t, engine, "restart@example.com")
	enqueue(t, engine, email, "recipient@example.net")
	_ = engine.Close()

	// Reopen the same database, as a restart does.
	reopened := openTestEngine(t, dsn)
	claimed, err := Claim(time.Now(), time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("Claim() after restart = %+v, %v; want the pending entry", claimed, err)
	}
	if got := loadEntry(t, reopened, claimed.Id); got.State != models.EmailQueueDelivering {
		t.Fatalf("entry state after restart claim = %d, want delivering", got.State)
	}
}
