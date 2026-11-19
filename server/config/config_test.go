package config

import (
	"testing"

	"github.com/noelbennettz/imail/utils/netutil"
)

func TestApplyDefaultsNormalizesNetworkModes(t *testing.T) {
	cases := []struct {
		listen string
		egress string
	}{
		{listen: "", egress: ""},
		{listen: "v4", egress: "v6"},
		{listen: "IPV6", egress: "IPV4"},
		{listen: "both", egress: "both"},
		{listen: "nonsense", egress: "nonsense"},
	}
	for _, tc := range cases {
		cfg := Config{ListenMode: tc.listen, EgressMode: tc.egress}
		cfg.ApplyDefaults()
		if cfg.ListenMode != netutil.NormalizeMode(tc.listen) {
			t.Fatalf("ListenMode = %q, want %q", cfg.ListenMode, netutil.NormalizeMode(tc.listen))
		}
		if cfg.EgressMode != netutil.NormalizeMode(tc.egress) {
			t.Fatalf("EgressMode = %q, want %q", cfg.EgressMode, netutil.NormalizeMode(tc.egress))
		}
	}
}

func TestApplyDefaultsKeepsFailedSigninLimitOn(t *testing.T) {
	// A config.json written before the setting existed has no field; the
	// decoded pointer is nil and the protection must stay on.
	var legacy Config
	legacy.ApplyDefaults()
	if legacy.LimitFailedSignins == nil {
		t.Fatal("LimitFailedSignins = nil after ApplyDefaults, want a value")
	}
	if !*legacy.LimitFailedSignins {
		t.Fatal("LimitFailedSignins = false for a config without the field, want true")
	}
	if !LoginLimiterEnabled() {
		t.Fatal("live limiter setting is off for a config without the field")
	}

	disabled := false
	explicit := Config{LimitFailedSignins: &disabled}
	explicit.ApplyDefaults()
	if explicit.LimitFailedSignins == nil || *explicit.LimitFailedSignins {
		t.Fatal("ApplyDefaults overrode an explicit false")
	}
	if LoginLimiterEnabled() {
		t.Fatal("live limiter setting is on after an explicit false")
	}

	// Leave the process-wide value enabled so other tests see the default.
	enabled := true
	enabledConfig := Config{LimitFailedSignins: &enabled}
	enabledConfig.ApplyDefaults()
}
