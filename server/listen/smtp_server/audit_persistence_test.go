package smtp_server

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	smtp "github.com/emersion/go-smtp"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	listservice "github.com/noelbennettz/imail/services/list"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestOutgoingSubmissionInsertFailureReturns451(t *testing.T) {
	engine, dsn := newQueueTestEngine(t)
	blocker := beginAuditReadTransaction(t, dsn)

	session := newAuditTestSession()
	err := session.deliverOutgoingEmail(newAuditTestEmail("insert-busy@example.com"))

	assertSMTPCode(t, err, 451)
	assertAuditCounts(t, engine, 0, 0)
	if rows := queueRows(t, engine); len(rows) != 0 {
		t.Fatalf("queue rows = %d, want 0 after a failed commit", len(rows))
	}

	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := session.deliverOutgoingEmail(newAuditTestEmail("insert-busy@example.com")); err != nil {
		t.Fatalf("submission after lock release failed: %v", err)
	}
	assertAuditCounts(t, engine, 1, 1)
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)
	if rows := queueRows(t, engine); len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1 after the commit", len(rows))
	}
}

func TestOutgoingSubmissionRetriesBusyInitialInsert(t *testing.T) {
	engine, dsn := newQueueTestEngine(t)
	blocker := beginAuditReadTransaction(t, dsn)
	released := make(chan struct{})
	go func() {
		time.Sleep(35 * time.Millisecond)
		_ = blocker.Rollback()
		close(released)
	}()

	err := newAuditTestSession().deliverOutgoingEmail(newAuditTestEmail("initial-retry@example.com"))
	<-released
	if err != nil {
		t.Fatalf("deliverOutgoingEmail() error = %v", err)
	}
	assertAuditCounts(t, engine, 1, 1)
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)
}

// TestOutgoingSubmissionQueuesLocalRecipientsToo is the durability fix: a
// mailbox this server hosts is queued like any other recipient, so the commit
// that acknowledges the message already covers it.
func TestOutgoingSubmissionQueuesLocalRecipientsToo(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	session.RawEmailData = []byte("From: sender@example.com\r\nSubject: local\r\n\r\nbody\r\n")
	email := newAuditTestEmail("local-queued@example.com")
	if err := session.deliverOutgoingEmail(email); err != nil {
		t.Fatalf("deliverOutgoingEmail() = %v, want success after the commit", err)
	}
	rows := queueRows(t, engine)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want one for the local recipient", len(rows))
	}
	recipients, err := queue.Recipients(&rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != "recipient@example.com" {
		t.Fatalf("queued recipients = %v, want the local mailbox", recipients)
	}
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)
}

func TestOutgoingAuditRelationInsertRollsBackEmail(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	if _, err := engine.Exec(`CREATE TRIGGER fail_user_email_insert
		BEFORE INSERT ON user_email
		BEGIN
			SELECT RAISE(ABORT, 'forced user_email insert failure');
		END`); err != nil {
		t.Fatal(err)
	}

	email := newAuditTestEmail("relation-failure@example.com")
	_, _, err := saveEmail(newAuditTestSession().Ctx, email.Size, nil, email, 1, int(consts.EmailTypeSend), nil, true, true, nil)
	if err == nil {
		t.Fatal("saveEmail() error = nil, want relation insert failure")
	}
	if email.MessageId != 0 {
		t.Fatalf("email.MessageId = %d, want 0 after rollback", email.MessageId)
	}
	assertAuditCounts(t, engine, 0, 0)
}

func TestIncomingAuditRelationInsertRollsBackEmail(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	recipient := &models.User{Account: "recipient", Name: "Recipient"}
	if _, err := engine.Insert(recipient); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Exec(`CREATE TRIGGER fail_incoming_user_email_insert
		BEFORE INSERT ON user_email
		BEGIN
			SELECT RAISE(ABORT, 'forced incoming user_email insert failure');
		END`); err != nil {
		t.Fatal(err)
	}

	from := &parsemail.User{Name: "External", EmailAddress: "external@example.net"}
	email := &parsemail.Email{
		From:   from,
		Sender: from,
		To:     []*parsemail.User{{EmailAddress: "recipient@example.com"}},
		Text:   []byte("body"),
		Size:   4,
		MsgID:  "incoming-relation-failure@example.net",
	}
	_, _, err := saveEmail(&context.Context{}, email.Size, nil, email, 0, int(consts.EmailTypeReceive), []string{"recipient@example.com"}, true, true, nil)
	if err == nil {
		t.Fatal("saveEmail() error = nil, want incoming relation insert failure")
	}
	assertAuditCounts(t, engine, 0, 0)
}

func TestSameMessageIDCreatesIndependentSubmissions(t *testing.T) {
	engine, _ := newQueueTestEngine(t)
	session := newAuditTestSession()
	for i := 0; i < 2; i++ {
		email := newAuditTestEmail("reused-message-id@example.com")
		email.Text = []byte(fmt.Sprintf("body %d", i))
		if err := session.deliverOutgoingEmail(email); err != nil {
			t.Fatalf("submission %d: %v", i+1, err)
		}
	}
	assertAuditCounts(t, engine, 2, 2)
	assertMessageIDCount(t, engine, "reused-message-id@example.com", 2)
	if rows := queueRows(t, engine); len(rows) != 2 {
		t.Fatalf("queue rows = %d, want one per submission", len(rows))
	}
}

func TestOutgoingPendingAuditMailboxVisibility(t *testing.T) {
	engine, _ := newAuditTestEngine(t)
	session := newAuditTestSession()
	email := newAuditTestEmail("pending-visibility@example.com")
	_, saved, err := saveEmail(session.Ctx, email.Size, nil, email, session.Ctx.UserID, int(consts.EmailTypeSend), nil, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("saved status = %d, want %d", saved.Status, consts.EmailStatusDeliveryPending)
	}
	assertAuditStatuses(t, engine, consts.EmailStatusDeliveryPending)

	outgoing, _ := listservice.GetEmailList(session.Ctx, dto.SearchTag{
		Type:    consts.EmailTypeSend,
		Status:  -1,
		GroupId: -1,
	}, "", false, 0, 10)
	found := false
	for _, item := range outgoing {
		if item.Id == saved.Id {
			found = true
			if item.Status != consts.EmailStatusDeliveryPending || !item.Error.Valid || item.Error.String != deliveryPendingError {
				t.Fatalf("outgoing pending audit = status:%d error:%#v", item.Status, item.Error)
			}
		}
	}
	if !found {
		t.Fatalf("pending audit email %d is missing from web outgoing list", saved.Id)
	}

	for _, mailbox := range []string{"INBOX", "Sent Messages"} {
		for _, item := range listservice.GetUEListByUID(session.Ctx, mailbox, 0, 0, nil) {
			if item.EmailID == saved.Id {
				t.Fatalf("pending audit email %d unexpectedly appears in IMAP mailbox %q", saved.Id, mailbox)
			}
		}
	}
}

func newAuditTestEngine(t *testing.T) (*xorm.Engine, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "audit.db") + "?_pragma=busy_timeout(20)"
	engine, err := xorm.NewEngine("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	engine.SetMaxIdleConns(1)
	if err = engine.Sync2(&models.User{}, &models.Email{}, &models.UserEmail{}); err != nil {
		engine.Close()
		t.Fatal(err)
	}
	if _, err = engine.Insert(&models.Email{Subject: "lock seed", MsgID: "lock-seed@example.com"}); err != nil {
		engine.Close()
		t.Fatal(err)
	}
	if _, err = engine.Insert(&models.UserEmail{UserID: 99, EmailID: 1}); err != nil {
		engine.Close()
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
	return engine, dsn
}

func beginAuditReadTransaction(t *testing.T, dsn string) *sql.Tx {
	t.Helper()
	blocker, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = blocker.Close() })
	tx, err := blocker.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM email").Scan(&count); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	return tx
}

func newAuditTestSession() *Session {
	return &Session{Ctx: &context.Context{UserID: 1, UserAccount: "sender", IsAdmin: true}}
}

func newAuditTestEmail(messageID string) *parsemail.Email {
	from := &parsemail.User{Name: "Sender", EmailAddress: "sender@example.com"}
	return &parsemail.Email{
		From:   from,
		Sender: from,
		// The local-domain recipient is queued like every other recipient.
		To:      []*parsemail.User{{EmailAddress: "recipient@example.com"}},
		Subject: "audit reliability",
		Text:    []byte("body"),
		Size:    4,
		MsgID:   messageID,
	}
}

func assertSMTPCode(t *testing.T, err error, want int) {
	t.Helper()
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) {
		t.Fatalf("error = %v (%T), want *smtp.SMTPError", err, err)
	}
	if smtpErr.Code != want {
		t.Fatalf("SMTP code = %d, want %d", smtpErr.Code, want)
	}
}

func assertAuditCounts(t *testing.T, engine *xorm.Engine, wantEmail, wantUserEmail int) {
	t.Helper()
	var emailCount, userEmailCount int
	if _, err := engine.SQL("SELECT count(*) FROM email WHERE id != 1").Get(&emailCount); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SQL("SELECT count(*) FROM user_email WHERE email_id != 1").Get(&userEmailCount); err != nil {
		t.Fatal(err)
	}
	if emailCount != wantEmail || userEmailCount != wantUserEmail {
		t.Fatalf("audit counts = email:%d user_email:%d, want email:%d user_email:%d", emailCount, userEmailCount, wantEmail, wantUserEmail)
	}
}

func assertAuditStatuses(t *testing.T, engine *xorm.Engine, want int8) {
	t.Helper()
	var emailStatus, userEmailStatus int8
	if _, err := engine.SQL("SELECT status FROM email WHERE id != 1 ORDER BY id DESC LIMIT 1").Get(&emailStatus); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SQL("SELECT status FROM user_email WHERE email_id != 1 ORDER BY email_id DESC LIMIT 1").Get(&userEmailStatus); err != nil {
		t.Fatal(err)
	}
	if emailStatus != want || userEmailStatus != want {
		t.Fatalf("audit statuses = email:%d user_email:%d, want %d", emailStatus, userEmailStatus, want)
	}
}

func assertMessageIDCount(t *testing.T, engine *xorm.Engine, messageID string, want int) {
	t.Helper()
	var count int
	if _, err := engine.SQL("SELECT count(*) FROM email WHERE msg_id=?", messageID).Get(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("rows for Message-ID %q = %d, want %d", messageID, count, want)
	}
}
