package queue

import (
	"testing"
	"time"

	"github.com/noelbennettz/imail/models"
	"xorm.io/xorm"
)

// finishEntry forces a row into a finished state at a chosen time. The
// timestamps are written with raw SQL because xorm maintains update_time itself.
func finishEntry(t *testing.T, engine *xorm.Engine, id int64, state int8, at time.Time) {
	t.Helper()
	if _, err := engine.Exec("update email_queue set state=?, update_time=? where id=?", state, at.UTC(), id); err != nil {
		t.Fatal(err)
	}
}

func countEntries(t *testing.T, engine *xorm.Engine) int64 {
	t.Helper()
	total, err := engine.Count(&models.EmailQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func TestPurgeKeepsOnlyRecentFinishedRows(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "purge@example.com")

	oldDelivered := enqueueAt(t, engine, email, now.Add(-8*24*time.Hour), "a@example.net")
	freshDelivered := enqueueAt(t, engine, email, now.Add(-6*24*time.Hour), "b@example.net")
	oldFailed := enqueueAt(t, engine, email, now.Add(-31*24*time.Hour), "c@example.net")
	freshFailed := enqueueAt(t, engine, email, now.Add(-29*24*time.Hour), "d@example.net")
	pending := enqueueAt(t, engine, email, now.Add(-40*24*time.Hour), "e@example.net")

	finishEntry(t, engine, oldDelivered.Id, models.EmailQueueDelivered, now.Add(-8*24*time.Hour))
	finishEntry(t, engine, freshDelivered.Id, models.EmailQueueDelivered, now.Add(-6*24*time.Hour))
	finishEntry(t, engine, oldFailed.Id, models.EmailQueueFailed, now.Add(-31*24*time.Hour))
	finishEntry(t, engine, freshFailed.Id, models.EmailQueueFailed, now.Add(-29*24*time.Hour))

	delivered, failed, err := Purge(now)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || failed != 1 {
		t.Fatalf("Purge() = %d delivered, %d failed; want 1 and 1", delivered, failed)
	}

	if _, ok := entryExists(t, engine, oldDelivered.Id); ok {
		t.Fatal("a delivered row past its retention survived")
	}
	if _, ok := entryExists(t, engine, oldFailed.Id); ok {
		t.Fatal("a failed row past its retention survived")
	}
	for _, id := range []int64{freshDelivered.Id, freshFailed.Id, pending.Id} {
		if _, ok := entryExists(t, engine, id); !ok {
			t.Fatalf("entry %d was deleted too early", id)
		}
	}
	if got := countEntries(t, engine); got != 3 {
		t.Fatalf("queue holds %d rows, want 3", got)
	}
}

func TestPurgeIsIdempotent(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "purge-twice@example.com")
	entry := enqueueAt(t, engine, email, now.Add(-9*24*time.Hour), "a@example.net")
	finishEntry(t, engine, entry.Id, models.EmailQueueDelivered, now.Add(-9*24*time.Hour))

	for i := 0; i < 2; i++ {
		if _, _, err := Purge(now); err != nil {
			t.Fatal(err)
		}
	}
	if got := countEntries(t, engine); got != 0 {
		t.Fatalf("queue holds %d rows after cleanup, want 0", got)
	}
}

func TestWorkerPurgesOnItsInterval(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "worker-purge@example.com")
	old := enqueueAt(t, engine, email, now.Add(-9*24*time.Hour), "a@example.net")
	finishEntry(t, engine, old.Id, models.EmailQueueDelivered, now.Add(-9*24*time.Hour))

	worker := newTestWorker(func() time.Time { return now })
	worker.PurgeInterval = time.Hour
	worker.maintain()
	if got := countEntries(t, engine); got != 0 {
		t.Fatalf("queue holds %d rows after the first maintenance, want 0", got)
	}

	// A second run inside the interval is a no-op; nothing is due and nothing
	// is deleted.
	fresh := enqueueAt(t, engine, email, now.Add(-9*24*time.Hour), "b@example.net")
	finishEntry(t, engine, fresh.Id, models.EmailQueueDelivered, now.Add(-9*24*time.Hour))
	worker.maintain()
	if got := countEntries(t, engine); got != 1 {
		t.Fatalf("maintenance ran again inside its interval: %d rows", got)
	}

	// After the interval the next run cleans up.
	now = now.Add(time.Hour)
	worker.maintain()
	if got := countEntries(t, engine); got != 0 {
		t.Fatalf("queue holds %d rows after the interval, want 0", got)
	}
}

func entryExists(t *testing.T, engine *xorm.Engine, id int64) (models.EmailQueue, bool) {
	t.Helper()
	var entry models.EmailQueue
	found, err := engine.ID(id).Get(&entry)
	if err != nil {
		t.Fatal(err)
	}
	return entry, found
}
