package smtp_server

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	"xorm.io/xorm"
)

// newQueueTestEngine extends the audit test engine with the outbound queue
// table.
func newQueueTestEngine(t *testing.T) (*xorm.Engine, string) {
	t.Helper()
	engine, dsn := newAuditTestEngine(t)
	if err := engine.Sync2(&models.EmailQueue{}); err != nil {
		t.Fatal(err)
	}
	return engine, dsn
}

func newQueueTestEmail(recipients ...string) *parsemail.Email {
	from := &parsemail.User{Name: "Sender", EmailAddress: "sender@example.com"}
	to := make([]*parsemail.User, 0, len(recipients))
	for _, address := range recipients {
		to = append(to, &parsemail.User{EmailAddress: address})
	}
	return &parsemail.Email{
		From:    from,
		Sender:  from,
		To:      to,
		Subject: "queued outbound",
		Text:    []byte("body"),
		Size:    4,
		MsgID:   "queued-outbound@example.com",
	}
}

func queueRows(t *testing.T, engine *xorm.Engine) []models.EmailQueue {
	t.Helper()
	var rows []models.EmailQueue
	if err := engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestOutboundRemoteRecipientIsQueuedAndAcknowledged(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	var deliveries atomic.Int32
	session := newAuditTestSession()
	session.RawEmailData = []byte("From: sender@example.com\r\nSubject: queued outbound\r\n\r\nbody\r\n")

	// One local and one remote recipient: the local copy is delivered inline,
	// the remote copy is queued, and the client is told the message was
	// accepted as soon as the queue row committed.
	err := session.deliverOutgoingEmail(newQueueTestEmail("local@example.com", "remote@example.net"),
		func(_ *context.Context, _ *parsemail.Email, recipients []*parsemail.User) (error, map[string]error) {
			deliveries.Add(1)
			if len(recipients) != 1 || recipients[0].EmailAddress != "local@example.com" {
				t.Errorf("inline recipients = %v, want only the local one", recipients)
			}
			return nil, nil
		})
	if err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v, want success after the queue commit", err)
	}
	if deliveries.Load() != 1 {
		t.Fatalf("inline deliveries = %d, want 1", deliveries.Load())
	}

	rows := queueRows(t, engine)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.State != models.EmailQueuePending || row.Attempts != 0 {
		t.Fatalf("queue row = state %d attempts %d, want pending/0", row.State, row.Attempts)
	}
	if row.EnvelopeFrom != "sender@example.com" {
		t.Fatalf("envelope sender = %q, want the message sender", row.EnvelopeFrom)
	}
	recipients, err := queue.Recipients(&row)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != "remote@example.net" {
		t.Fatalf("queued recipients = %v, want only the remote one", recipients)
	}

	var stored models.Email
	if _, err := engine.ID(row.EmailID).Get(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.RawMIME, "Subject: queued outbound") {
		t.Fatalf("stored MIME = %q, want the message body to be durable", stored.RawMIME)
	}
	// The audit row stays pending until the worker finishes.
	if stored.Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("email status = %d, want pending", stored.Status)
	}
}

func TestOutboundQueueInsertFailureFailsTheTransaction(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	if _, err := engine.Exec(`CREATE TRIGGER fail_queue_insert
		BEFORE INSERT ON email_queue
		BEGIN
			SELECT RAISE(ABORT, 'forced queue insert failure');
		END`); err != nil {
		t.Fatal(err)
	}

	session := newAuditTestSession()
	session.RawEmailData = []byte("From: sender@example.com\r\n\r\nbody\r\n")
	err := session.deliverOutgoingEmail(newQueueTestEmail("remote@example.net"),
		func(*context.Context, *parsemail.Email, []*parsemail.User) (error, map[string]error) {
			t.Error("inline delivery ran although the queue insert failed")
			return nil, nil
		})
	assertSMTPCode(t, err, 451)

	// The email row is rolled back with the queue row: the client sees a
	// temporary failure and retries the whole message.
	assertAuditCounts(t, engine, 0, 0)
	if rows := queueRows(t, engine); len(rows) != 0 {
		t.Fatalf("queue rows = %d, want none after the rollback", len(rows))
	}
}

func TestOutboundLocalRecipientsStayInline(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	var deliveries atomic.Int32
	session := newAuditTestSession()
	err := session.deliverOutgoingEmail(newQueueTestEmail("first@example.com", "second@example.com"),
		func(_ *context.Context, _ *parsemail.Email, recipients []*parsemail.User) (error, map[string]error) {
			deliveries.Add(1)
			if len(recipients) != 2 {
				t.Errorf("inline recipients = %d, want both local recipients", len(recipients))
			}
			return nil, nil
		})
	if err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v", err)
	}
	if deliveries.Load() != 1 {
		t.Fatalf("inline deliveries = %d, want 1", deliveries.Load())
	}
	if rows := queueRows(t, engine); len(rows) != 0 {
		t.Fatalf("queue rows = %d, want none for a local-only message", len(rows))
	}
	assertAuditStatuses(t, engine, consts.EmailStatusSent)
}

func TestOutboundQueuedMessageKeepsInlineFailureOutOfTheSMTPReply(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	err := session.deliverOutgoingEmail(newQueueTestEmail("local@example.com", "remote@example.net"),
		func(*context.Context, *parsemail.Email, []*parsemail.User) (error, map[string]error) {
			return errors.New("local delivery failed"), map[string]error{"example.com": fmt.Errorf("boom")}
		})
	// The remote copy is queued: returning an error here would make the client
	// retry and duplicate it, so the client gets 250 and the audit row records
	// the inline failure.
	if err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v, want success", err)
	}
	if rows := queueRows(t, engine); len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1", len(rows))
	}
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)
	assertAuditError(t, engine, "local delivery failed: local delivery failed", true)
}

func TestOutboundRemoteOnlyMessageIsQueuedWithoutInlineDelivery(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	err := session.deliverOutgoingEmail(newQueueTestEmail("remote@example.net"),
		func(*context.Context, *parsemail.Email, []*parsemail.User) (error, map[string]error) {
			t.Error("no inline delivery is expected for remote-only recipients")
			return nil, nil
		})
	if err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v, want success", err)
	}
	rows := queueRows(t, engine)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1", len(rows))
	}
	var stored models.Email
	if _, err := engine.ID(rows[0].EmailID).Get(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("email status = %d, want pending until the worker delivers", stored.Status)
	}
}
