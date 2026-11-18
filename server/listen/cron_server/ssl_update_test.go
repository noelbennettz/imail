package cron_server

import (
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/netutil"
)

// TestProbeURLFollowsListenMode is the regression guard for certificate
// renewal on IPv6-only hosts: probing 127.0.0.1 never succeeds when the HTTP
// listener is bound to [::] with v6only set.
func TestProbeURLFollowsListenMode(t *testing.T) {
	oldConfig := config.Instance
	t.Cleanup(func() { config.Instance = oldConfig })

	config.Instance = &config.Config{ListenMode: netutil.ModeIPv4, HttpPort: 8080}
	if netutil.SupportsIPv4() {
		if got, want := probeURL(), "http://127.0.0.1:8080/api/ping"; got != want {
			t.Fatalf("probeURL() = %q, want %q", got, want)
		}
	}

	if netutil.SupportsIPv6() {
		config.Instance = &config.Config{ListenMode: netutil.ModeIPv6, HttpPort: 8080}
		if got, want := probeURL(), "http://[::1]:8080/api/ping"; got != want {
			t.Fatalf("probeURL() = %q, want %q", got, want)
		}
	}

	config.Instance = &config.Config{HttpPort: 8080}
	if netutil.SupportsIPv4() {
		if got, want := probeURL(), "http://127.0.0.1:8080/api/ping"; got != want {
			t.Fatalf("probeURL() = %q, want %q", got, want)
		}
	}
}
