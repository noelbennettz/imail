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
