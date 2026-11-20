package setup

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestToken(t *testing.T) *SetupToken {
	t.Helper()
	token, err := NewSetupToken(false)
	if err != nil {
		t.Fatalf("NewSetupToken: %v", err)
	}
	return token
}

func TestGeneratedTokenShape(t *testing.T) {
	token := newTestToken(t)
	value := token.Token()
	if len(value) != tokenLength {
		t.Fatalf("token length = %d, want %d", len(value), tokenLength)
	}
	for _, char := range value {
		if !strings.ContainsRune(tokenAlphabet, char) {
			t.Fatalf("token contains %q, which is outside [A-Za-z0-9]", char)
		}
	}
	if !token.Enabled() {
		t.Fatal("a fresh token must be enabled")
	}
}

func TestTokenIsUniquePerStart(t *testing.T) {
	first := newTestToken(t).Token()
	second := newTestToken(t).Token()
	if first == second {
		t.Fatal("two starts produced the same token")
	}
}

// An initialised deployment must not generate or print anything: there is no
// token and the setup API stays closed.
func TestInitialisedDeploymentHasNoToken(t *testing.T) {
	token, err := NewSetupToken(true)
	if err != nil {
		t.Fatalf("NewSetupToken: %v", err)
	}
	if token.Token() != "" {
		t.Fatalf("initialised deployment produced token %q", token.Token())
	}
	if token.Enabled() {
		t.Fatal("setup API must be closed on an initialised deployment")
	}
	if got := token.Verify("127.0.0.1:1234", ""); got != VerifyClosed {
		t.Fatalf("Verify on initialised deployment = %v, want VerifyClosed", got)
	}
}

func TestVerifyMissingAndWrongToken(t *testing.T) {
	token := newTestToken(t)
	if got := token.Verify("192.0.2.10:1111", ""); got != VerifyRejected {
		t.Fatalf("missing token = %v, want VerifyRejected", got)
	}
	wrong := strings.Repeat("x", tokenLength)
	if got := token.Verify("192.0.2.10:1111", wrong); got != VerifyRejected {
		t.Fatalf("wrong token = %v, want VerifyRejected", got)
	}
	short := token.Token()[:tokenLength-1]
	if got := token.Verify("192.0.2.10:1111", short); got != VerifyRejected {
		t.Fatalf("truncated token = %v, want VerifyRejected", got)
	}
}

func TestVerifyCorrectToken(t *testing.T) {
	token := newTestToken(t)
	if got := token.Verify("192.0.2.10:1111", token.Token()); got != VerifyAllowed {
		t.Fatalf("correct token = %v, want VerifyAllowed", got)
	}
}

func TestTokenEquals(t *testing.T) {
	if !tokenEquals("abcdefghijklmnopqrstuvwx", "abcdefghijklmnopqrstuvwx") {
		t.Fatal("equal tokens compared unequal")
	}
	if tokenEquals("abcdefghijklmnopqrstuvwx", "abcdefghijklmnopqrstuvwX") {
		t.Fatal("differing tokens compared equal")
	}
	if tokenEquals("abcdefghijklmnopqrstuvwx", "abcdefghijklmnopqrstuvw") {
		t.Fatal("truncated token compared equal")
	}
	if tokenEquals("abcdefghijklmnopqrstuvwx", "") {
		t.Fatal("empty token compared equal")
	}
}

// Five failures from one address block that address for fifteen minutes; a
// different address is unaffected.
func TestPerAddressBlock(t *testing.T) {
	token := newTestToken(t)
	wrong := strings.Repeat("y", tokenLength)
	for i := 0; i < maxFailuresPerAddress; i++ {
		if got := token.Verify("192.0.2.20:2222", wrong); got != VerifyRejected {
			t.Fatalf("failure %d = %v, want VerifyRejected", i+1, got)
		}
	}
	if got := token.Verify("192.0.2.20:3333", token.Token()); got != VerifyRateLimited {
		t.Fatalf("blocked address with correct token = %v, want VerifyRateLimited", got)
	}
	if got := token.Verify("192.0.2.21:2222", token.Token()); got != VerifyAllowed {
		t.Fatalf("different address = %v, want VerifyAllowed", got)
	}
}

func TestBlockExpires(t *testing.T) {
	token := newTestToken(t)
	now := time.Now()
	token.now = func() time.Time { return now }

	wrong := strings.Repeat("y", tokenLength)
	for i := 0; i < maxFailuresPerAddress; i++ {
		token.Verify("192.0.2.30:1", wrong)
	}
	if got := token.Verify("192.0.2.30:1", token.Token()); got != VerifyRateLimited {
		t.Fatalf("blocked = %v, want VerifyRateLimited", got)
	}

	now = now.Add(failureWindow + time.Second)
	token.now = func() time.Time { return now }
	if got := token.Verify("192.0.2.30:1", token.Token()); got != VerifyAllowed {
		t.Fatalf("after the window = %v, want VerifyAllowed", got)
	}
}

// Failures spread over several addresses must not shut the API down for
// everyone: there is no global counter any more, because a stranger could
// otherwise keep a legitimate administrator out of their own first setup by
// burning the shared budget.
func TestFailuresAcrossAddressesKeepTokenUsable(t *testing.T) {
	token := newTestToken(t)
	wrong := strings.Repeat("z", tokenLength)

	failures := 0
	for address := 0; failures < 20; address++ {
		remote := "198.51.100." + strconv.Itoa(address+1) + ":1234"
		for i := 0; i < maxFailuresPerAddress-1; i++ {
			if got := token.Verify(remote, wrong); got != VerifyRejected {
				t.Fatalf("failure %d from %s = %v, want VerifyRejected", failures+1, remote, got)
			}
			failures++
		}
	}
	if failures < 20 {
		t.Fatalf("only %d failures driven, want at least 20", failures)
	}

	// The per-address block still works: a fifth failure from one of those
	// addresses locks it, and only that address.
	if got := token.Verify("198.51.100.1:1234", wrong); got != VerifyRejected {
		t.Fatalf("fifth failure = %v, want VerifyRejected", got)
	}
	if got := token.Verify("198.51.100.1:1234", token.Token()); got != VerifyRateLimited {
		t.Fatalf("blocked address = %v, want VerifyRateLimited", got)
	}
	if got := token.Verify("203.0.113.99:1234", token.Token()); got != VerifyAllowed {
		t.Fatalf("correct token from a fresh address = %v, want VerifyAllowed", got)
	}
}

// The token is dropped from memory once setup finishes, and the API stays
// closed from then on.
func TestClearClosesSetupAPI(t *testing.T) {
	token := newTestToken(t)
	value := token.Token()
	token.Clear()
	if token.Token() != "" {
		t.Fatalf("token still in memory after Clear: %q", token.Token())
	}
	if token.Enabled() {
		t.Fatal("setup API still enabled after Clear")
	}
	if got := token.Verify("192.0.2.40:1", value); got != VerifyClosed {
		t.Fatalf("Verify after Clear = %v, want VerifyClosed", got)
	}
}

func TestInitTokenAndClearToken(t *testing.T) {
	token, err := InitToken(false)
	if err != nil {
		t.Fatalf("InitToken: %v", err)
	}
	if CurrentToken() != token {
		t.Fatal("CurrentToken did not return the initialised token")
	}
	ClearToken()
	if CurrentToken() != nil {
		t.Fatal("CurrentToken still set after ClearToken")
	}
	if token.Token() != "" || token.Enabled() {
		t.Fatal("token survived ClearToken")
	}
}

func TestVerifyResultHTTPStatus(t *testing.T) {
	cases := map[VerifyResult]int{
		VerifyAllowed:     http.StatusOK,
		VerifyRejected:    http.StatusForbidden,
		VerifyClosed:      http.StatusForbidden,
		VerifyRateLimited: http.StatusTooManyRequests,
	}
	for result, want := range cases {
		if got := result.HTTPStatus(); got != want {
			t.Fatalf("HTTPStatus(%v) = %d, want %d", result, got, want)
		}
	}
}
