// Package netutil centralizes socket binding and TLS policy for every
// listener (HTTP, SMTP, IMAP, POP3).
package netutil

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Listen modes supported by the server.
const (
	ModeBoth = "both"
	ModeIPv4 = "ipv4"
	ModeIPv6 = "ipv6"
)

// NormalizeMode validates a configured listen mode and falls back to both.
func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeIPv4, "v4", "4":
		return ModeIPv4
	case ModeIPv6, "v6", "6":
		return ModeIPv6
	default:
		return ModeBoth
	}
}

var (
	stackOnce sync.Once
	hasIPv4   bool
	hasIPv6   bool
)

// SupportsIPv4 reports whether this host can bind an IPv4 loopback socket.
func SupportsIPv4() bool {
	stackOnce.Do(detectStacks)
	return hasIPv4
}

// SupportsIPv6 reports whether this host can bind an IPv6 loopback socket.
func SupportsIPv6() bool {
	stackOnce.Do(detectStacks)
	return hasIPv6
}

func detectStacks() {
	hasIPv4 = canListen("tcp4", "127.0.0.1:0")
	hasIPv6 = canListen("tcp6", "[::1]:0")
}

func canListen(network, address string) bool {
	listener, err := net.Listen(network, address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// EffectiveMode downgrades a configured mode to a family this host actually
// has. A host without IPv6 must not fail because the config says "ipv6".
func EffectiveMode(mode string) string {
	return effectiveMode(mode, SupportsIPv4(), SupportsIPv6())
}

func effectiveMode(mode string, ipv4Available, ipv6Available bool) string {
	switch NormalizeMode(mode) {
	case ModeIPv4:
		if !ipv4Available && ipv6Available {
			return ModeIPv6
		}
		return ModeIPv4
	case ModeIPv6:
		if !ipv6Available && ipv4Available {
			return ModeIPv4
		}
		return ModeIPv6
	default:
		return ModeBoth
	}
}

// Network returns the tcp network name used for net.Listen.
func Network(mode string) string {
	return networkFor(EffectiveMode(mode))
}

func networkFor(mode string) string {
	switch NormalizeMode(mode) {
	case ModeIPv4:
		return "tcp4"
	case ModeIPv6:
		return "tcp6"
	default:
		return "tcp"
	}
}

// Host returns the wildcard host used when binding.
// An empty host keeps Go's dual-stack wildcard behaviour.
func Host(mode string) string {
	switch EffectiveMode(mode) {
	case ModeIPv4:
		return "0.0.0.0"
	case ModeIPv6:
		return "::"
	default:
		return ""
	}
}

// Address builds a bind address such as ":25", "0.0.0.0:25" or "[::]:25".
func Address(mode string, port int) string {
	host := Host(mode)
	if host == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

// Loopback returns a loopback address reachable in the configured mode.
func Loopback(mode string) string {
	if EffectiveMode(mode) == ModeIPv6 {
		return "::1"
	}
	return "127.0.0.1"
}

// LoopbackAddress returns host:port for internal health probes.
func LoopbackAddress(mode string, port int) string {
	return net.JoinHostPort(Loopback(mode), fmt.Sprintf("%d", port))
}

// DialTimeout dials addr over the family selected by mode. The mode is the
// egress mode: "both" keeps Go's Happy Eyeballs behaviour, "ipv4"/"ipv6"
// force one family so a host with a broken stack does not hang.
func DialTimeout(mode, addr string, timeout time.Duration) (net.Conn, error) {
	return DialContext(context.Background(), mode, addr, timeout)
}

// DialContext is the context-aware variant of DialTimeout.
func DialContext(ctx context.Context, mode, addr string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, Network(mode), addr)
}

// ProbeClient is used for loopback readiness probes. It ignores proxy
// environment variables, which would otherwise send 127.0.0.1/[::1] through
// an HTTP proxy.
var ProbeClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
	},
}

// BuildTLSConfigMulti applies the TLS 1.2/1.3 policy and strict SNI
// validation, selecting a per-host certificate when one is available and
// falling back to the shared certificate otherwise.
func BuildTLSConfigMulti(defaultCert *tls.Certificate, byName map[string]*tls.Certificate, minVersion, maxVersion uint16, hosts []string, enforceSNI bool) *tls.Config {
	allowed := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if host != "" {
			allowed[host] = struct{}{}
		}
	}

	cfg := &tls.Config{
		MinVersion: minVersion,
		MaxVersion: maxVersion,
	}
	if defaultCert != nil {
		cfg.Certificates = []tls.Certificate{*defaultCert}
	}

	if !enforceSNI && len(byName) == 0 {
		return cfg
	}

	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		name := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
		if name == "" {
			if enforceSNI {
				return nil, fmt.Errorf("TLS SNI is required")
			}
			return nil, nil
		}
		if _, ok := allowed[name]; !ok && enforceSNI {
			return nil, fmt.Errorf("TLS SNI %q is not configured for this server", name)
		}
		if cert, ok := byName[name]; ok && cert != nil {
			return &tls.Config{
				Certificates: []tls.Certificate{*cert},
				MinVersion:   minVersion,
				MaxVersion:   maxVersion,
			}, nil
		}
		return nil, nil
	}

	return cfg
}

// BuildTLSConfig applies the TLS 1.2/1.3 policy and optional strict SNI
// validation shared by all TLS listeners.
//
// When enforceSNI is true the handshake is refused if the client sends no
// server name or a name outside the configured host set. This is the
// behaviour required for the internal deployment: connecting by bare IP or
// with a foreign domain must not complete a TLS exchange.
func BuildTLSConfig(cert tls.Certificate, minVersion, maxVersion uint16, hosts []string, enforceSNI bool) *tls.Config {
	return BuildTLSConfigMulti(&cert, nil, minVersion, maxVersion, hosts, enforceSNI)
}

// TLSVersion parses a configured TLS version string. Only 1.2 and 1.3 are
// accepted; anything else falls back to the supplied default.
func TLSVersion(value string, fallback uint16) uint16 {
	switch strings.TrimSpace(value) {
	case "1.2", "TLS1.2", "tls1.2":
		return tls.VersionTLS12
	case "1.3", "TLS1.3", "tls1.3":
		return tls.VersionTLS13
	default:
		return fallback
	}
}
