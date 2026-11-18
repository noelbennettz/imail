package netutil

import (
	"crypto/tls"
	"strings"
	"testing"
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
	cases := map[string]string{
		ModeBoth: "",
		ModeIPv4: "0.0.0.0",
		ModeIPv6: "::",
	}
	for mode, host := range cases {
		if got := Host(mode); got != host {
			t.Fatalf("Host(%q) = %q, want %q", mode, got, host)
		}
	}
	if addr := Address(ModeIPv4, 25); addr != "0.0.0.0:25" {
		t.Fatalf("unexpected IPv4 address %q", addr)
	}
	if addr := Address(ModeIPv6, 25); addr != "[::]:25" {
		t.Fatalf("unexpected IPv6 address %q", addr)
	}
	if !strings.Contains(LoopbackAddress(ModeIPv6, 80), "::1") {
		t.Fatalf("unexpected IPv6 loopback")
	}
}
