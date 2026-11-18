package netutil

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestBuildTLSConfigEnforcesSNI(t *testing.T) {
	cfg := BuildTLSConfig(tls.Certificate{}, tls.VersionTLS12, tls.VersionTLS13,
		[]string{"mail.example.test", "smtp.example.test"}, true)

	if cfg.MinVersion != tls.VersionTLS12 || cfg.MaxVersion != tls.VersionTLS13 {
		t.Fatalf("unexpected TLS bounds: %x-%x", cfg.MinVersion, cfg.MaxVersion)
	}
	if cfg.GetConfigForClient == nil {
		t.Fatal("expected SNI callback")
	}

	if _, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: ""}); err == nil {
		t.Fatal("empty SNI must be refused")
	}
	if _, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "evil.test"}); err == nil {
		t.Fatal("unknown SNI must be refused")
	}
	for _, name := range []string{"mail.example.test", "MAIL.Example.Test.", "smtp.example.test"} {
		if _, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: name}); err != nil {
			t.Fatalf("valid SNI %q refused: %v", name, err)
		}
	}
}

func TestBuildTLSConfigCanDisableSNI(t *testing.T) {
	cfg := BuildTLSConfig(tls.Certificate{}, tls.VersionTLS12, tls.VersionTLS13, nil, false)
	if cfg.GetConfigForClient != nil {
		t.Fatal("SNI callback should be nil when enforcement is disabled")
	}
}

func TestBuildTLSConfigMultiSelectsPerHostCertificate(t *testing.T) {
	fallback := tls.Certificate{Certificate: [][]byte{{1}}}
	dedicated := tls.Certificate{Certificate: [][]byte{{2}}}
	cfg := BuildTLSConfigMulti(&fallback, map[string]*tls.Certificate{
		"smtp.example.test": &dedicated,
	}, tls.VersionTLS12, tls.VersionTLS13, []string{"example.test", "smtp.example.test"}, true)

	if cfg.GetConfigForClient == nil {
		t.Fatal("expected SNI callback")
	}
	got, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "smtp.example.test"})
	if err != nil {
		t.Fatalf("dedicated SNI refused: %v", err)
	}
	if len(got.Certificates) != 1 || string(got.Certificates[0].Certificate[0]) != string([]byte{2}) {
		t.Fatal("expected the per-host certificate")
	}
	got, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "example.test"})
	if err != nil {
		t.Fatalf("fallback SNI refused: %v", err)
	}
	if got == nil {
		// A nil config means "use the outer configuration", which carries the
		// shared certificate.
		got = cfg
	}
	if len(got.Certificates) != 1 || string(got.Certificates[0].Certificate[0]) != string([]byte{1}) {
		t.Fatal("expected the fallback certificate")
	}
	if _, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "evil.test"}); err == nil {
		t.Fatal("unknown SNI must still be refused in strict mode")
	}
}

func TestBuildTLSConfigMultiAllowsEmptySNIWhenNotStrict(t *testing.T) {
	fallback := tls.Certificate{Certificate: [][]byte{{1}}}
	cfg := BuildTLSConfigMulti(&fallback, nil, tls.VersionTLS12, tls.VersionTLS13, nil, false)
	if cfg.GetConfigForClient != nil {
		t.Fatal("SNI callback should be nil without enforcement or per-host certs")
	}
}

func TestAddressModes(t *testing.T) {
	if got := Host(ModeBoth); got != "" {
		t.Fatalf("Host(both) = %q, want the empty dual-stack wildcard", got)
	}
	if SupportsIPv4() {
		if got := Host(ModeIPv4); got != "0.0.0.0" {
			t.Fatalf("Host(ipv4) = %q, want 0.0.0.0", got)
		}
		if addr := Address(ModeIPv4, 25); addr != "0.0.0.0:25" {
			t.Fatalf("unexpected IPv4 address %q", addr)
		}
	}
	if SupportsIPv6() {
		if got := Host(ModeIPv6); got != "::" {
			t.Fatalf("Host(ipv6) = %q, want ::", got)
		}
		if addr := Address(ModeIPv6, 25); addr != "[::]:25" {
			t.Fatalf("unexpected IPv6 address %q", addr)
		}
		if !strings.Contains(LoopbackAddress(ModeIPv6, 80), "::1") {
			t.Fatalf("unexpected IPv6 loopback")
		}
	}
}

func TestEffectiveModeFallsBackWhenFamilyIsMissing(t *testing.T) {
	cases := []struct {
		name string
		mode string
		v4   bool
		v6   bool
		want string
	}{
		{name: "ipv4 host, ipv4 requested", mode: ModeIPv4, v4: true, want: ModeIPv4},
		{name: "ipv4-only host, ipv6 requested", mode: ModeIPv6, v4: true, want: ModeIPv4},
		{name: "ipv6-only host, ipv4 requested", mode: ModeIPv4, v6: true, want: ModeIPv6},
		{name: "ipv6 host, ipv6 requested", mode: ModeIPv6, v6: true, want: ModeIPv6},
		{name: "both on a dual-stack host", mode: ModeBoth, v4: true, v6: true, want: ModeBoth},
		{name: "both on an ipv4-only host", mode: ModeBoth, v4: true, want: ModeBoth},
		{name: "unknown value", mode: "banana", v4: true, v6: true, want: ModeBoth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveMode(tc.mode, tc.v4, tc.v6); got != tc.want {
				t.Fatalf("effectiveMode(%q, %v, %v) = %q, want %q", tc.mode, tc.v4, tc.v6, got, tc.want)
			}
		})
	}
}

// startAccepting binds a listener on the wildcard address of the given mode and
// returns it together with its port.
func startAccepting(t *testing.T, mode string) (net.Listener, int) {
	t.Helper()
	listener, err := net.Listen(Network(mode), Address(mode, 0))
	if err != nil {
		t.Skipf("cannot listen in %s mode here: %v", mode, err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return listener, listener.Addr().(*net.TCPAddr).Port
}

func dials(t *testing.T, network, address string) bool {
	t.Helper()
	conn, err := net.DialTimeout(network, address, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// TestListenersServeTheConfiguredFamily is the ingress contract: after
// binding in a mode, a client of that family can connect, and a client of the
// other family cannot (unless the mode is dual-stack).
func TestListenersServeTheConfiguredFamily(t *testing.T) {
	if !SupportsIPv4() {
		t.Skip("no IPv4 loopback on this host")
	}
	_, v4Port := startAccepting(t, ModeIPv4)
	if got := Network(ModeIPv4); got != "tcp4" {
		t.Fatalf("Network(ipv4) = %q", got)
	}
	if !dials(t, "tcp4", LoopbackAddress(ModeIPv4, v4Port)) {
		t.Fatal("IPv4 listener did not accept an IPv4 connection")
	}
	if SupportsIPv6() && dials(t, "tcp6", net.JoinHostPort("::1", itoa(v4Port))) {
		t.Fatal("IPv4-only listener accepted an IPv6 connection")
	}

	if !SupportsIPv6() {
		t.Skip("no IPv6 loopback on this host")
	}
	_, v6Port := startAccepting(t, ModeIPv6)
	if got := Network(ModeIPv6); got != "tcp6" {
		t.Fatalf("Network(ipv6) = %q", got)
	}
	if !dials(t, "tcp6", LoopbackAddress(ModeIPv6, v6Port)) {
		t.Fatal("IPv6 listener did not accept an IPv6 connection")
	}
	if dials(t, "tcp4", net.JoinHostPort("127.0.0.1", itoa(v6Port))) {
		t.Fatal("IPv6-only listener accepted an IPv4 connection")
	}

	_, bothPort := startAccepting(t, ModeBoth)
	if !dials(t, "tcp4", net.JoinHostPort("127.0.0.1", itoa(bothPort))) {
		t.Fatal("dual-stack listener did not accept an IPv4 connection")
	}
	if !dials(t, "tcp6", net.JoinHostPort("::1", itoa(bothPort))) {
		t.Fatal("dual-stack listener did not accept an IPv6 connection")
	}
}

func TestDialTimeoutForcesTheConfiguredFamily(t *testing.T) {
	if !SupportsIPv6() || !SupportsIPv4() {
		t.Skip("both address families are required")
	}
	_, v6Port := startAccepting(t, ModeIPv6)

	if _, err := DialTimeout(ModeIPv6, LoopbackAddress(ModeIPv6, v6Port), time.Second); err != nil {
		t.Fatalf("IPv6 dial failed: %v", err)
	}
	if _, err := DialTimeout(ModeIPv4, LoopbackAddress(ModeIPv6, v6Port), time.Second); err == nil {
		t.Fatal("IPv4-forced dial reached an IPv6-only listener")
	}
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
