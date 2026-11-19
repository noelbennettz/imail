package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/utils/context"
)

// useTempConfig points the config package at a fresh directory for one test.
func useTempConfig(t *testing.T) {
	t.Helper()
	oldRoot := config.ROOT_PATH
	oldInstance := config.Instance
	oldEnabled := config.LoginLimiterEnabled()
	config.ROOT_PATH = t.TempDir() + "/"
	enabled := true
	config.Instance = &config.Config{Domain: "example.com", Domains: []string{"example.com"}, LimitFailedSignins: &enabled}
	config.SetLoginLimiterEnabled(true)
	if err := config.WriteConfig(config.Instance); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		config.ROOT_PATH = oldRoot
		config.Instance = oldInstance
		config.SetLoginLimiterEnabled(oldEnabled)
	})
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var decoded response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return decoded
}

func TestSaveSecuritySettingsPersistsAndApplies(t *testing.T) {
	useTempConfig(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/admin/security/save", strings.NewReader(`{"limitFailedSignins":false}`))
	SaveSecuritySettings(&context.Context{UserID: 1, IsAdmin: true}, recorder, request)
	if decoded := decodeResponse(t, recorder); decoded.ErrorNo != 0 {
		t.Fatalf("save response = %+v, want success", decoded)
	}
	if config.LoginLimiterEnabled() {
		t.Fatal("limiter still enabled after saving false")
	}
	stored, err := config.ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if stored.LimitFailedSignins == nil || *stored.LimitFailedSignins {
		t.Fatal("config.json does not record limitFailedSignins=false")
	}
	if config.Instance.LimitFailedSignins == nil || *config.Instance.LimitFailedSignins {
		t.Fatal("Instance was not updated with the saved value")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/api/admin/security", nil)
	SecuritySettings(&context.Context{UserID: 1, IsAdmin: true}, recorder, request)
	decoded := decodeResponse(t, recorder)
	data, ok := decoded.Data.(map[string]any)
	if !ok || data["limitFailedSignins"] != false {
		t.Fatalf("security settings response = %+v, want limitFailedSignins=false", decoded)
	}
}

func TestSecuritySettingsRequiresAdmin(t *testing.T) {
	useTempConfig(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/admin/security/save", strings.NewReader(`{"limitFailedSignins":false}`))
	SaveSecuritySettings(&context.Context{UserID: 2}, recorder, request)
	if decoded := decodeResponse(t, recorder); decoded.ErrorNo != response.NoAccessPrivileges {
		t.Fatalf("non-admin save response = %+v, want no access privileges", decoded)
	}
	if !config.LoginLimiterEnabled() {
		t.Fatal("non-admin request changed the setting")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/api/admin/security", nil)
	SecuritySettings(&context.Context{UserID: 2}, recorder, request)
	if decoded := decodeResponse(t, recorder); decoded.ErrorNo != response.NoAccessPrivileges {
		t.Fatalf("non-admin read response = %+v, want no access privileges", decoded)
	}
}

func TestSaveSecuritySettingsRejectsMissingField(t *testing.T) {
	useTempConfig(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/admin/security/save", strings.NewReader(`{}`))
	SaveSecuritySettings(&context.Context{UserID: 1, IsAdmin: true}, recorder, request)
	if decoded := decodeResponse(t, recorder); decoded.ErrorNo != response.ParamsError {
		t.Fatalf("response = %+v, want a parameter error", decoded)
	}
	if !config.LoginLimiterEnabled() {
		t.Fatal("a request without the field changed the setting")
	}
}
