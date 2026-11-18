package id

import (
	"net"
	"testing"
)

func TestAddressPrefixHandlesBothFamilies(t *testing.T) {
	if got := addressPrefix("203.0.113.7"); len(got) != 4 || net.IP(got).String() != "203.0.113.7" {
		t.Fatalf("IPv4 prefix = %v", got)
	}
	if got := addressPrefix("2001:db8::7"); len(got) != 4 {
		t.Fatalf("IPv6 prefix length = %d, want 4", len(got))
	}
	if got := addressPrefix("not-an-ip"); len(got) != 4 {
		t.Fatalf("fallback prefix length = %d, want 4", len(got))
	}
}

func TestGenLogIDHasFixedWidthPrefix(t *testing.T) {
	// 4 address bytes (8 hex) + timestamp (8) + nanos (4) + pid (4) + random (6) + "b0".
	if got := GenLogID(); len(got) != 32 {
		t.Fatalf("GenLogID() length = %d, want 32 (id=%q)", len(got), got)
	}
}
