package ip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func startAddressServer(t *testing.T, network, address, body string) string {
	t.Helper()
	listener, err := net.Listen(network, address)
	if err != nil {
		t.Skipf("cannot listen on %s %s: %v", network, address, err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server.URL
}

// TestDetectOnlyAcceptsTheForcedFamily proves the A/AAAA guidance cannot be
// polluted by a dual-stack endpoint answering on the other family.
func TestDetectOnlyAcceptsTheForcedFamily(t *testing.T) {
	url4 := startAddressServer(t, "tcp4", "127.0.0.1:0", "203.0.113.7")
	client4 := familyClient("tcp4", 2*time.Second)
	if got := detect(client4, url4); got != "203.0.113.7" {
		t.Fatalf("IPv4 detect = %q, want 203.0.113.7", got)
	}

	url6 := startAddressServer(t, "tcp6", "[::1]:0", "2001:db8::7")
	client6 := familyClient("tcp6", 2*time.Second)
	if got := detect(client6, url6); got != "2001:db8::7" {
		t.Fatalf("IPv6 detect = %q, want 2001:db8::7", got)
	}

	// An IPv4-only client must not reach an IPv6-only endpoint.
	if got := detect(client4, url6); got != "" {
		t.Fatalf("IPv4 client returned %q from an IPv6-only endpoint", got)
	}
	// An IPv6-only client must not reach an IPv4-only endpoint.
	if got := detect(client6, url4); got != "" {
		t.Fatalf("IPv6 client returned %q from an IPv4-only endpoint", got)
	}
}

// TestDetectFallsBackToTheNextURL covers the common deployment where the
// primary lookup service is unreachable for one family.
func TestDetectFallsBackToTheNextURL(t *testing.T) {
	url4 := startAddressServer(t, "tcp4", "127.0.0.1:0", "203.0.113.9")
	client4 := familyClient("tcp4", 500*time.Millisecond)
	if got := detect(client4, "http://127.0.0.1:1/", url4); got != "203.0.113.9" {
		t.Fatalf("detect fallback = %q, want 203.0.113.9", got)
	}
}

func TestDetectRejectsNonAddressBodies(t *testing.T) {
	url4 := startAddressServer(t, "tcp4", "127.0.0.1:0", "<html>error</html>")
	client4 := familyClient("tcp4", 2*time.Second)
	if got := detect(client4, url4); got != "" {
		t.Fatalf("detect returned %q for a non-address body", got)
	}
}
