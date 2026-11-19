package email

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	"xorm.io/xorm"
)

// initWebmailTestDKIM writes a throwaway signing key and initialises the
// package-level DKIM signer, which BuildBytes needs.
func initWebmailTestDKIM(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "dkim.priv")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(file, &pem.Block{Type: "PRIVATE KEY", Bytes: encoded}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	config.Instance.DkimPrivateKeyPath = path
	parsemail.Init()
}

func openWebmailTestEngine(t *testing.T, dsn string) *xorm.Engine {
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

func useWebmailTestDatabase(t *testing.T) (*xorm.Engine, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "webmail.db")
	engine := openWebmailTestEngine(t, dsn)
	initWebmailTestDKIM(t)
	return engine, dsn
}

func decodeWebmailResponse(t *testing.T, recorder *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var decoded response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return decoded
}

func webmailRequestBody(t *testing.T, to ...string) string {
	t.Helper()
	recipients := make([]map[string]string, 0, len(to))
	for _, address := range to {
		recipients = append(recipients, map[string]string{"name": "", "email": address})
	}
	body, err := json.Marshal(map[string]any{
		"from":    map[string]string{"name": "Sender", "email": "sender@example.com"},
		"to":      recipients,
		"subject": "composed in the webmail",
		"text":    "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestWebmailSendQueuesRemoteRecipients(t *testing.T) {
	engine, _ := useWebmailTestDatabase(t)
	ctx := &context.Context{UserID: 1, UserAccount: "sender", UserName: "Sender", Lang: "en"}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/email/send", strings.NewReader(webmailRequestBody(t, "remote@other.example")))
	Send(ctx, recorder, request)
	if decoded := decodeWebmailResponse(t, recorder); decoded.ErrorNo != 0 {
		t.Fatalf("response = %+v, want success after the commit", decoded)
	}

	var stored models.Email
	found, err := engine.Desc("id").Get(&stored)
	if err != nil || !found {
		t.Fatalf("email row = found %t err %v", found, err)
	}
	if stored.Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("email status = %d, want pending for a queued message", stored.Status)
	}
	if !strings.Contains(stored.RawMIME, "Subject: composed in the webmail") {
		t.Fatalf("stored MIME = %q, want the rendered message for the worker", stored.RawMIME)
	}
	var relation models.UserEmail
	found, err = engine.Where("email_id = ?", stored.Id).Get(&relation)
	if err != nil || !found {
		t.Fatalf("user_email row = found %t err %v", found, err)
	}
	if relation.Status != consts.EmailStatusDeliveryPending {
		t.Fatalf("user_email status = %d, want pending", relation.Status)
	}

	var rows []models.EmailQueue
	if err := engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want one per remote domain", len(rows))
	}
	recipients, err := queue.Recipients(&rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != "remote@other.example" {
		t.Fatalf("queued recipients = %v, want the remote address", recipients)
	}
	if rows[0].EnvelopeFrom != "sender@example.com" {
		t.Fatalf("envelope sender = %q", rows[0].EnvelopeFrom)
	}
}

func TestWebmailQueuedMessageSurvivesRestart(t *testing.T) {
	engine, dsn := useWebmailTestDatabase(t)
	ctx := &context.Context{UserID: 1, UserAccount: "sender", UserName: "Sender", Lang: "en"}
	recorder := httptest.NewRecorder()
	Send(ctx, recorder, httptest.NewRequest("POST", "/api/email/send",
		strings.NewReader(webmailRequestBody(t, "one@first.example", "two@second.example"))))
	if decoded := decodeWebmailResponse(t, recorder); decoded.ErrorNo != 0 {
		t.Fatalf("response = %+v, want success", decoded)
	}
	var rows []models.EmailQueue
	if err := engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("queue rows = %d, want one per remote domain", len(rows))
	}

	// A restart closes and reopens the database; the worker claims the rows
	// again without any user action.
	_ = engine.Close()
	openWebmailTestEngine(t, dsn)
	claimed := 0
	for {
		entry, err := queue.Claim(time.Now(), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if entry == nil {
			break
		}
		claimed++
	}
	if claimed != 2 {
		t.Fatalf("claimed after restart = %d, want both queued domains", claimed)
	}
}

func TestWebmailSendReportsCommitFailure(t *testing.T) {
	engine, _ := useWebmailTestDatabase(t)
	if _, err := engine.Exec(`CREATE TRIGGER fail_webmail_queue_insert
		BEFORE INSERT ON email_queue
		BEGIN
			SELECT RAISE(ABORT, 'forced queue insert failure');
		END`); err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{UserID: 1, UserAccount: "sender", UserName: "Sender", Lang: "en"}
	recorder := httptest.NewRecorder()
	Send(ctx, recorder, httptest.NewRequest("POST", "/api/email/send",
		strings.NewReader(webmailRequestBody(t, "remote@other.example"))))
	if decoded := decodeWebmailResponse(t, recorder); decoded.ErrorNo == 0 {
		t.Fatalf("response = %+v, want a failure when the queue row cannot be written", decoded)
	}
	// The whole transaction rolled back: no sent copy, no queue row.
	emails, err := engine.Table(&models.Email{}).Count()
	if err != nil {
		t.Fatal(err)
	}
	queued, err := engine.Table(&models.EmailQueue{}).Count()
	if err != nil {
		t.Fatal(err)
	}
	if emails != 0 || queued != 0 {
		t.Fatalf("rows after the failed commit = email %d queue %d, want 0/0", emails, queued)
	}
}
