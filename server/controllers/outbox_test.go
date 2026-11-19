package controllers

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"xorm.io/xorm"
)

func newOutboxTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	engine.SetMaxIdleConns(1)
	if err = engine.Sync2(&models.EmailQueue{}); err != nil {
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

func TestOutboxStatusRequiresAdmin(t *testing.T) {
	newOutboxTestEngine(t)
	recorder := httptest.NewRecorder()
	OutboxStatus(&context.Context{UserID: 2}, recorder, httptest.NewRequest("GET", "/api/admin/outbox", nil))
	if decoded := decodeResponse(t, recorder); decoded.ErrorNo != response.NoAccessPrivileges {
		t.Fatalf("non-admin response = %+v, want no access privileges", decoded)
	}
}

func TestOutboxStatusReportsCounts(t *testing.T) {
	engine := newOutboxTestEngine(t)
	now := time.Now().UTC()
	for _, state := range []int8{models.EmailQueuePending, models.EmailQueueDelivering, models.EmailQueueFailed, models.EmailQueueDelivered} {
		entry := models.EmailQueue{
			EmailID:      1,
			EnvelopeFrom: "sender@example.com",
			Recipients:   `["recipient@example.net"]`,
			State:        state,
			NextAttempt:  now,
			CreateTime:   now,
			UpdateTime:   now,
		}
		if _, err := engine.Insert(&entry); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	OutboxStatus(&context.Context{UserID: 1, IsAdmin: true}, recorder, httptest.NewRequest("GET", "/api/admin/outbox", nil))
	decoded := decodeResponse(t, recorder)
	if decoded.ErrorNo != 0 {
		t.Fatalf("response = %+v, want success", decoded)
	}
	data, ok := decoded.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want an object", decoded.Data)
	}
	// JSON numbers decode into float64.
	if data["pending"] != float64(2) || data["failed"] != float64(1) {
		t.Fatalf("counts = pending %v failed %v, want 2/1", data["pending"], data["failed"])
	}
}
