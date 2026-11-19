package smtp_server

import (
	"strings"
	"testing"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
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

func TestOutboundRecipientsAreQueuedAndAcknowledged(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	session.RawEmailData = []byte("From: sender@example.com\r\nSubject: queued outbound\r\n\r\nbody\r\n")

	// One local and one remote recipient: both are queued, and the client is
	// told the message was accepted as soon as the transaction commits.
	if err := session.deliverOutgoingEmail(newQueueTestEmail("local@example.com", "remote@example.net")); err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v, want success after the queue commit", err)
	}

	rows := queueRows(t, engine)
	if len(rows) != 2 {
		t.Fatalf("queue rows = %d, want one per recipient", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
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
		if len(recipients) != 1 {
			t.Fatalf("queued recipients = %v, want exactly one address", recipients)
		}
		seen[recipients[0]] = true
	}
	if !seen["local@example.com"] || !seen["remote@example.net"] {
		t.Fatalf("queued recipients = %v, want the local and the remote address", seen)
	}

	var stored models.Email
	if _, err := engine.ID(rows[0].EmailID).Get(&stored); err != nil {
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
	err := session.deliverOutgoingEmail(newQueueTestEmail("remote@example.net"))
	assertSMTPCode(t, err, 451)

	// The email row is rolled back with the queue row: the client sees a
	// temporary failure and retries the whole message.
	assertAuditCounts(t, engine, 0, 0)
	if rows := queueRows(t, engine); len(rows) != 0 {
		t.Fatalf("queue rows = %d, want none after the rollback", len(rows))
	}
}

func TestOutboundLocalOnlyMessageIsQueued(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	if err := session.deliverOutgoingEmail(newQueueTestEmail("first@example.com", "second@example.com")); err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v", err)
	}
	rows := queueRows(t, engine)
	if len(rows) != 2 {
		t.Fatalf("queue rows = %d, want one per local recipient", len(rows))
	}
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)
}

func TestOutboundQueueRowPerRecipient(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	session.RawEmailData = []byte("From: sender@example.com\r\n\r\nbody\r\n")
	if err := session.deliverOutgoingEmail(newQueueTestEmail("a@one.example", "b@two.example", "c@one.example")); err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v", err)
	}
	rows := queueRows(t, engine)
	if len(rows) != 3 {
		t.Fatalf("queue rows = %d, want one per recipient", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		recipients, err := queue.Recipients(&row)
		if err != nil {
			t.Fatal(err)
		}
		if len(recipients) != 1 {
			t.Fatalf("queued recipients = %v, want exactly one address", recipients)
		}
		seen[recipients[0]] = true
	}
	if len(seen) != 3 {
		t.Fatalf("queue recipients = %v, want three distinct addresses", seen)
	}
}
