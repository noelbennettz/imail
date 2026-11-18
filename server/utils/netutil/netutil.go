// Package netutil centralizes socket binding and TLS policy for every
// listener (HTTP, SMTP, IMAP, POP3).
package netutil

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
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

// Network returns the tcp network name used for net.Listen.
func Network(mode string) string {
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
	switch NormalizeMode(mode) {
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
	if NormalizeMode(mode) == ModeIPv6 {
		return "::1"
	}
	return "127.0.0.1"
}

// LoopbackAddress returns host:port for internal health probes.
func LoopbackAddress(mode string, port int) string {
	return net.JoinHostPort(Loopback(mode), fmt.Sprintf("%d", port))
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
