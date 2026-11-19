package queue

import (
	"errors"
	"testing"
	"time"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/models"
	"xorm.io/xorm"
)

func TestListSeparatesPendingAndFailed(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "list@example.com")

	pending := enqueueAt(t, engine, email, now, "a@example.net")
	failed := enqueueAt(t, engine, email, now, "b@example.net")
	if _, err := engine.Exec("update email_queue set state=?, last_error=?, attempts=3 where id=?",
		models.EmailQueueFailed, "550 mailbox unavailable", failed.Id); err != nil {
		t.Fatal(err)
	}

	entries, total, err := List("pending", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(entries) != 1 || entries[0].ID != pending.Id {
		t.Fatalf("pending list = %d rows (total %d), want entry %d", len(entries), total, pending.Id)
	}
	if entries[0].State != "pending" || len(entries[0].Recipients) != 1 || entries[0].Recipients[0] != "a@example.net" {
		t.Fatalf("unexpected pending view: %+v", entries[0])
	}

	entries, total, err = List("failed", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("failed list = %d rows (total %d), want 1", len(entries), total)
	}
	if entries[0].LastError != "550 mailbox unavailable" || entries[0].Attempts != 3 || entries[0].State != "failed" {
		t.Fatalf("unexpected failed view: %+v", entries[0])
	}

	if _, _, err := List("everything", 1, 20); err == nil {
		t.Fatal("an unknown filter was accepted")
	}
}

func TestListPaginates(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "page@example.com")
	for i := 0; i < 5; i++ {
		enqueueAt(t, engine, email, now, "a@example.net")
	}

	first, total, err := List("pending", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(first) != 2 {
		t.Fatalf("page 1 = %d rows, total %d; want 2 and 5", len(first), total)
	}
	last, _, err := List("pending", 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 1 {
		t.Fatalf("page 3 = %d rows, want 1", len(last))
	}
	if first[0].ID <= first[1].ID {
		t.Fatalf("rows are not newest first: %d then %d", first[0].ID, first[1].ID)
	}
}

func TestRetryReturnsAFailedRowToTheQueue(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "retry@example.com")
	entry := enqueueAt(t, engine, email, now.Add(-96*time.Hour), "a@example.net")
	if err := MarkFailed(nil, &entry, "550 mailbox unavailable", now); err != nil {
		t.Fatal(err)
	}
	if status := loadEmail(t, engine, email.Id).Status; status != consts.EmailStatusFail {
		t.Fatalf("email status after the failure = %d, want failed", status)
	}

	retryAt := now.Add(time.Hour)
	if err := Retry(entry.Id, retryAt); err != nil {
		t.Fatal(err)
	}
	got := loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueuePending {
		t.Fatalf("state after retry = %s, want pending", StateName(got.State))
	}
	if got.Attempts != 0 {
		t.Fatalf("attempts after retry = %d, want 0", got.Attempts)
	}
	if !got.NextAttempt.Equal(retryAt) {
		t.Fatalf("next attempt = %s, want %s", got.NextAttempt, retryAt)
	}
	if !got.CreateTime.Equal(retryAt) {
		t.Fatalf("create time = %s, want the retry time %s", got.CreateTime, retryAt)
	}
	if got.LastError.Valid && got.LastError.String != "" {
		t.Fatalf("last error after retry = %q, want empty", got.LastError.String)
	}
	if status := loadEmail(t, engine, email.Id).Status; status != consts.EmailStatusDeliveryPending {
		t.Fatalf("email status after retry = %d, want delivery pending", status)
	}

	// The worker picks it up straight away and the retried row can be sent.
	worker := newTestWorker(func() time.Time { return retryAt })
	worker.deliver = success
	if processed := worker.Drain(); processed != 1 {
		t.Fatalf("Drain() after the retry = %d, want 1", processed)
	}
	if got := loadEntry(t, engine, entry.Id); got.State != models.EmailQueueDelivered {
		t.Fatalf("state after the retried delivery = %s, want delivered", StateName(got.State))
	}
}

func TestRetryRejectsWrongStateAndUnknownRows(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "retry-state@example.com")
	pending := enqueueAt(t, engine, email, now, "a@example.net")

	if err := Retry(pending.Id, now); !errors.Is(err, ErrEntryState) {
		t.Fatalf("Retry(pending) = %v, want ErrEntryState", err)
	}
	if err := Retry(9999, now); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Retry(unknown) = %v, want ErrEntryNotFound", err)
	}
}

func TestDeleteRemovesOnlyFinishedRows(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "delete@example.com")

	delivered := enqueueAt(t, engine, email, now, "a@example.net")
	if err := MarkDelivered(nil, &delivered, now); err != nil {
		t.Fatal(err)
	}
	failed := enqueueAt(t, engine, email, now, "b@example.net")
	if err := MarkFailed(nil, &failed, "550 no such user", now); err != nil {
		t.Fatal(err)
	}
	pending := enqueueAt(t, engine, email, now, "c@example.net")

	if err := Delete(delivered.Id); err != nil {
		t.Fatalf("Delete(delivered): %v", err)
	}
	if err := Delete(failed.Id); err != nil {
		t.Fatalf("Delete(failed): %v", err)
	}
	if err := Delete(pending.Id); !errors.Is(err, ErrEntryState) {
		t.Fatalf("Delete(pending) = %v, want ErrEntryState", err)
	}
	if err := Delete(9999); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Delete(unknown) = %v, want ErrEntryNotFound", err)
	}

	if _, found := queueEntryExists(t, engine, delivered.Id); found {
		t.Fatal("a delivered row was not deleted")
	}
	if _, found := queueEntryExists(t, engine, failed.Id); found {
		t.Fatal("a failed row was not deleted")
	}
	if _, found := queueEntryExists(t, engine, pending.Id); !found {
		t.Fatal("a pending row was deleted")
	}
}

func queueEntryExists(t *testing.T, engine *xorm.Engine, id int64) (models.EmailQueue, bool) {
	t.Helper()
	var entry models.EmailQueue
	found, err := engine.ID(id).Get(&entry)
	if err != nil {
		t.Fatal(err)
	}
	return entry, found
}
