package rule

import (
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

// TestRemoteForwardIsQueued proves a rule forwarding to another domain is
// stored as an email plus queue rows instead of being delivered synchronously
// inside the inbound transaction.
func TestRemoteForwardIsQueued(t *testing.T) {
	oldConfig := config.Instance
	oldDB := db.Instance
	t.Cleanup(func() {
		config.Instance = oldConfig
		db.Instance = oldDB
	})
	config.Instance = &config.Config{Domain: "example.com", Domains: []string{"example.com"}}

	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	db.Instance = engine
	if err := engine.Sync2(&models.User{}, &models.Email{}, &models.UserEmail{}, &models.EmailQueue{}); err != nil {
		t.Fatal(err)
	}

	owner := &models.User{ID: 1, Account: "owner", Name: "Owner"}
	email := &parsemail.Email{
		From:    &parsemail.User{EmailAddress: "friend@elsewhere.example"},
		Subject: "hello",
		Text:    []byte("body"),
	}
	raw := []byte("From: friend@elsewhere.example\r\nSubject: hello\r\n\r\nbody\r\n")
	if err := doForward(&context.Context{}, email, "target@remote.example", owner, raw); err != nil {
		t.Fatalf("doForward() = %v", err)
	}

	var stored []models.Email
	if err := engine.Find(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("email rows = %d, want the forwarded message", len(stored))
	}
	if stored[0].Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("forwarded email status = %d, want delivery pending", stored[0].Status)
	}
	if string(stored[0].RawMIME) != string(raw) {
		t.Fatalf("forwarded MIME was not stored verbatim")
	}

	var rows []models.EmailQueue
	if err := engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want one for the forward target", len(rows))
	}
	if rows[0].EnvelopeFrom != "owner@example.com" {
		t.Fatalf("envelope sender = %q", rows[0].EnvelopeFrom)
	}
	recipients, err := queue.Recipients(&rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != "target@remote.example" {
		t.Fatalf("queued recipients = %v, want the forward target", recipients)
	}
	// No mailbox relation: the original message is already in the owner's
	// mailbox, so the forward must not duplicate it there.
	links, err := engine.Count(&models.UserEmail{})
	if err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatalf("mailbox relations = %d, want none", links)
	}
}
