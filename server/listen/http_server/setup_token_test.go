package http_server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/setup"
)

type setupFixture struct {
	server *httptest.Server
	token  *setup.SetupToken
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	token, err := setup.NewSetupToken(false)
	if err != nil {
		t.Fatalf("NewSetupToken: %v", err)
	}
	server := httptest.NewServer(newSetupMux(token))
	t.Cleanup(server.Close)
	return &setupFixture{server: server, token: token}
}

// do sends a request to the setup handler. An empty token means "no header".
func (f *setupFixture) do(t *testing.T, method, path, token, body string) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(setup.TokenHeader, token)
	}
	resp, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, data
}

func (f *setupFixture) setupCall(t *testing.T, token string) (*http.Response, []byte) {
	t.Helper()
	return f.do(t, http.MethodPost, setupAPIPath, token, `{"action":"get","step":"database"}`)
}

// Missing and wrong tokens are both 403 and the response is byte-identical, so
// the answer does not reveal which case applies.
func TestSetupTokenMissingAndWrongAreIndistinguishable(t *testing.T) {
	f := newSetupFixture(t)

	missing, missingBody := f.setupCall(t, "")
	if missing.StatusCode != http.StatusForbidden {
		t.Fatalf("missing token status = %d, want 403", missing.StatusCode)
	}
	wrong, wrongBody := f.setupCall(t, strings.Repeat("a", 24))
	if wrong.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", wrong.StatusCode)
	}
	if string(missingBody) != string(wrongBody) {
		t.Fatalf("bodies differ: %q vs %q", missingBody, wrongBody)
	}
	var parsed response.Response
	if err := json.Unmarshal(missingBody, &parsed); err != nil {
		t.Fatalf("unmarshal 403 body: %v", err)
	}
	if parsed.ErrorNo != response.InvalidSetupToken {
		t.Fatalf("errorNo = %d, want %d", parsed.ErrorNo, response.InvalidSetupToken)
	}
}

// The correct token lets the setup API run.
func TestSetupTokenAllowsSetupAPI(t *testing.T) {
	f := newSetupFixture(t)

	resp, body := f.setupCall(t, f.token.Token())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var parsed response.Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if parsed.ErrorNo == response.InvalidSetupToken {
		t.Fatalf("setup API rejected the correct token: %s", body)
	}
}

// The first screen verifies the token before it shows the wizard.
func TestSetupTokenVerifyEndpoint(t *testing.T) {
	f := newSetupFixture(t)

	resp, body := f.do(t, http.MethodPost, setupVerifyPath, "", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("verify without token = %d, want 403 (body %s)", resp.StatusCode, body)
	}

	resp, body = f.do(t, http.MethodPost, setupVerifyPath, f.token.Token(), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify with token = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var parsed response.Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if parsed.ErrorNo != 0 {
		t.Fatalf("verify errorNo = %d, want 0", parsed.ErrorNo)
	}
}

// Five failures from one address lock that address out: the next attempt is
// 429 even when the token is right.
func TestSetupTokenPerAddressRateLimit(t *testing.T) {
	f := newSetupFixture(t)
	wrong := strings.Repeat("b", 24)
	for i := 0; i < 5; i++ {
		resp, _ := f.setupCall(t, wrong)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("failure %d = %d, want 403", i+1, resp.StatusCode)
		}
	}
	resp, _ := f.setupCall(t, f.token.Token())
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after five failures = %d, want 429", resp.StatusCode)
	}
}

// Failures spread over several addresses must not lock the API for everyone:
// after twenty failures from other addresses the correct token still gets 200.
func TestSetupTokenNotLockedOutByOtherAddresses(t *testing.T) {
	f := newSetupFixture(t)
	wrong := strings.Repeat("c", 24)
	for address := 0; address < 5; address++ {
		remote := "198.51.100." + strconv.Itoa(address+1) + ":1234"
		for i := 0; i < 4; i++ {
			if got := f.token.Verify(remote, wrong); got != setup.VerifyRejected {
				t.Fatalf("failure %d from %s = %v, want VerifyRejected", address*4+i+1, remote, got)
			}
		}
	}

	resp, body := f.setupCall(t, f.token.Token())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("after failures from other addresses = %d, want 200 (body %s)", resp.StatusCode, body)
	}
}

// Only the static client, the health check and the minimal status endpoint are
// reachable without a token.
func TestSetupUnauthenticatedPaths(t *testing.T) {
	f := newSetupFixture(t)

	resp, body := f.do(t, http.MethodGet, pingPath, "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "pong") {
		t.Fatalf("GET %s = %d %s, want 200 pong", pingPath, resp.StatusCode, body)
	}

	resp, body = f.do(t, http.MethodGet, setupStatusPath, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", setupStatusPath, resp.StatusCode)
	}
	var parsed struct {
		ErrorNo int `json:"errorNo"`
		Data    struct {
			Initialized   bool `json:"initialized"`
			RequiresToken bool `json:"requires_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if parsed.ErrorNo != 0 || parsed.Data.Initialized || !parsed.Data.RequiresToken {
		t.Fatalf("status = %s, want initialized=false and requires_token=true", body)
	}

	resp, body = f.do(t, http.MethodGet, "/", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(string(body)), "<html") {
		t.Fatalf("GET / did not serve the client: %s", body[:min(len(body), 80)])
	}

	// Any other API path stays closed as well.
	resp, _ = f.do(t, http.MethodGet, "/api/email/list", "", "")
	if resp.StatusCode == http.StatusOK {
		t.Fatal("unknown setup API path answered 200")
	}
}

// After initialization the token is gone from memory and every setup endpoint
// is closed for good.
func TestSetupEndpointsClosedAfterInitialisation(t *testing.T) {
	f := newSetupFixture(t)
	issued := f.token.Token()
	f.token.Clear()

	resp, _ := f.setupCall(t, issued)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("setup API after init = %d, want 403", resp.StatusCode)
	}
	resp, _ = f.do(t, http.MethodPost, setupVerifyPath, issued, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("verify after init = %d, want 403", resp.StatusCode)
	}

	oldInit := config.IsInit
	config.IsInit = true
	t.Cleanup(func() { config.IsInit = oldInit })

	_, body := f.do(t, http.MethodGet, setupStatusPath, "", "")
	if !strings.Contains(string(body), `"initialized":true`) || !strings.Contains(string(body), `"requires_token":false`) {
		t.Fatalf("status after init = %s", body)
	}
}

// A process that starts on an initialised deployment generates no token and
// keeps the setup API shut.
func TestSetupMuxOnInitialisedStart(t *testing.T) {
	token, err := setup.NewSetupToken(true)
	if err != nil {
		t.Fatalf("NewSetupToken: %v", err)
	}
	if token.Token() != "" {
		t.Fatalf("initialised start generated token %q", token.Token())
	}
	server := httptest.NewServer(newSetupMux(token))
	t.Cleanup(server.Close)

	f := &setupFixture{server: server, token: token}
	resp, _ := f.setupCall(t, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("setup API on initialised start = %d, want 403", resp.StatusCode)
	}
}
