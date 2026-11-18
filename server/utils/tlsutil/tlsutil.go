// Package tlsutil builds listener TLS configurations from the active
// configuration, including per-domain certificates.
package tlsutil

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/netutil"
)

// DomainCertDir returns the directory that holds a domain's own certificate.
func DomainCertDir(domain string) string {
	return filepath.Join(config.ROOT_PATH, "config", "ssl", "domains", domain)
}

// DomainCertPaths returns the key and certificate paths for a domain.
func DomainCertPaths(domain string) (string, string) {
	dir := DomainCertDir(domain)
	return filepath.Join(dir, "private.key"), filepath.Join(dir, "public.crt")
}

// LoadDomainCertificate loads the per-domain certificate if present.
func LoadDomainCertificate(domain string) (*tls.Certificate, bool) {
	keyPath, certPath := DomainCertPaths(domain)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, false
	}
	_ = os.Chmod(keyPath, 0o600)
	return &cert, true
}

// ServerConfig loads the shared certificate plus every per-domain certificate
// and returns a TLS configuration that selects by SNI.
func ServerConfig() *tls.Config {
	cfg := config.Instance
	minVersion, maxVersion := cfg.TLSVersions()

	var fallback *tls.Certificate
	if cert, err := tls.LoadX509KeyPair(cfg.SSLPublicKeyPath, cfg.SSLPrivateKeyPath); err == nil {
		fallback = &cert
		_ = os.Chmod(cfg.SSLPrivateKeyPath, 0o600)
	}

	byName := map[string]*tls.Certificate{}
	for _, domain := range cfg.Domains {
		cert, ok := LoadDomainCertificate(domain)
		if !ok {
			continue
		}
		byName[domain] = cert
		byName["smtp."+domain] = cert
		byName["imap."+domain] = cert
		byName["pop."+domain] = cert
		web := strings.ToLower(strings.TrimSuffix(cfg.WebDomain, "."))
		if web == domain || strings.HasSuffix(web, "."+domain) {
			byName[web] = cert
		}
		if fallback == nil {
			fallback = cert
		}
	}

	return netutil.BuildTLSConfigMulti(fallback, byName, minVersion, maxVersion,
		cfg.TLSHostnames(), !cfg.DisableSNICheck)
}
