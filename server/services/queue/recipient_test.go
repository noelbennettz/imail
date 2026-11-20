package queue

import (
	"errors"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
)

// TestPartialRecipientSuccessIsNeverResent proves the per-recipient rows: a
// domain that accepted one address is never contacted again because another
// address at the same domain was rejected.
func TestPartialRecipientSuccessIsNeverResent(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "partial@example.com")

	entries, err := Entries(email.Id, email.MsgID, "sender@example.com",
		[]string{"accepted@remote.example", "rejected@remote.example"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if _, err := engine.Insert(&entries[i]); err != nil {
			t.Fatal(err)
		}
	}

	var deliveries []string
	worker := newTestWorker(func() time.Time { return now })
	worker.deliver = func(_ *context.Context, entry *models.EmailQueue, _ *models.Email) (error, map[string]error) {
		recipients, err := Recipients(entry)
		if err != nil {
			return err, nil
		}
		deliveries = append(deliveries, recipients[0])
		if recipients[0] == "accepted@remote.example" {
			return nil, map[string]error{"remote.example": nil}
		}
		return errors.New("delivery failed for rejected@remote.example"), map[string]error{
			"remote.example": &textproto.Error{Code: 550, Msg: "no such user"},
		}
	}
	if processed := worker.Drain(); processed != 2 {
		t.Fatalf("Drain() = %d, want both rows", processed)
	}
	if len(deliveries) != 2 {
		t.Fatalf("deliveries = %v, want both recipients exactly once", deliveries)
	}

	// Nothing is due any more: the accepted recipient is not tried again.
	if processed := worker.Drain(); processed != 0 {
		t.Fatalf("Drain() again = %d, want nothing to resend", processed)
	}

	states := map[string]int8{}
	var rows []models.EmailQueue
	if err := engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		recipients, err := Recipients(&rows[i])
		if err != nil {
			t.Fatal(err)
		}
		states[recipients[0]] = rows[i].State
	}
	if states["accepted@remote.example"] != models.EmailQueueDelivered {
		t.Fatalf("accepted recipient state = %d, want delivered", states["accepted@remote.example"])
	}
	if states["rejected@remote.example"] != models.EmailQueueFailed {
		t.Fatalf("rejected recipient state = %d, want failed", states["rejected@remote.example"])
	}
}

// TestLocalRecipientSurvivesRestart is the durability test for local mail: the
// row for a mailbox this server hosts is in the database, so reopening the
// database after a crash still delivers it.
func TestLocalRecipientSurvivesRestart(t *testing.T) {
	engine, dsn := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "local-restart@example.com")
	entries, err := Entries(email.Id, email.MsgID, "sender@example.com", []string{"local@example.com"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if _, err := engine.Insert(&entries[0]); err != nil {
		t.Fatal(err)
	}

	// A restart: close the database and open the same file again.
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := reopenTestEngine(t, dsn)

	entry, err := Claim(now, DefaultLease)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("the local queue row was not claimed after the restart")
	}
	recipients, err := Recipients(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != "local@example.com" {
		t.Fatalf("claimed recipients = %v, want the local mailbox", recipients)
	}

	if err := MarkDelivered(nil, entry, now); err != nil {
		t.Fatal(err)
	}
	if got := loadEntry(t, reopened, entry.Id); got.State != models.EmailQueueDelivered {
		t.Fatalf("state after delivery = %d, want delivered", got.State)
	}
	if status := loadEmail(t, reopened, email.Id).Status; status != consts.EmailStatusSent {
		t.Fatalf("email status = %d, want sent", status)
	}
}

// TestFinalFailureStoresADeliveryStatusNotification checks the bounce: a local
// envelope sender gets a small DSN in its mailbox when a row finally fails, and
// a remote sender gets nothing (no backscatter).
func TestFinalFailureStoresADeliveryStatusNotification(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	sender := models.User{Account: "sender", Name: "Sender"}
	if _, err := engine.Insert(&sender); err != nil {
		t.Fatal(err)
	}
	email := insertEmail(t, engine, "bounce@example.com")
	entry := enqueueAt(t, engine, email, now, "missing@remote.example")

	if err := MarkFailed(nil, &entry, "550 mailbox unavailable", now); err != nil {
		t.Fatal(err)
	}

	var bounces []models.Email
	if err := engine.Where("from_address = ?", "MAILER-DAEMON@example.com").Find(&bounces); err != nil {
		t.Fatal(err)
	}
	if len(bounces) != 1 {
		t.Fatalf("bounces = %d, want 1", len(bounces))
	}
	bounce := bounces[0]
	if bounce.Subject != "Delivery Status Notification (Failure)" {
		t.Fatalf("bounce subject = %q", bounce.Subject)
	}
	if !strings.Contains(bounce.RawMIME, "missing@remote.example") ||
		!strings.Contains(bounce.RawMIME, "550 mailbox unavailable") {
		t.Fatalf("bounce body does not carry the recipient and the reason: %s", bounce.RawMIME)
	}
	if !strings.Contains(bounce.To, "sender@example.com") {
		t.Fatalf("bounce To = %q, want the envelope sender", bounce.To)
	}
	var relations []models.UserEmail
	if err := engine.Where("email_id = ?", bounce.Id).Find(&relations); err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 || relations[0].UserID != sender.ID {
		t.Fatalf("bounce mailbox relations = %+v, want the sender's mailbox", relations)
	}
	if relations[0].Status != consts.EmailStatusWait {
		t.Fatalf("bounce relation status = %d, want the Inbox status", relations[0].Status)
	}
}

func TestNoBounceIsSentToARemoteSender(t *testing.T) {
	engine, _ := newTestEngine(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	email := insertEmail(t, engine, "no-bounce@example.com")
	entry := enqueueAt(t, engine, email, now, "missing@remote.example")
	if err := MarkFailed(nil, &entry, "550 mailbox unavailable", now); err != nil {
		t.Fatal(err)
	}

	count, err := engine.Where("from_address like ?", "MAILER-DAEMON%").Count(&models.Email{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("bounces to a remote sender = %d, want 0", count)
	}
}
