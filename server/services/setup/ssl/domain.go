package ssl

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/signal"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/tlsutil"
	log "github.com/sirupsen/logrus"
)

// DomainCertificateInfo reports the certificate used for one domain.
type DomainCertificateInfo struct {
	CertificateInfo
	Domain string `json:"domain"`
	Shared bool   `json:"shared"`
	Mode   string `json:"mode"`
}

func readCertificateInfo(keyPath, certPath string) (CertificateInfo, bool) {
	info := CertificateInfo{}
	tlsCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		info.Error = err.Error()
		return info, false
	}
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		info.Error = err.Error()
		return info, false
	}
	info.Configured = true
	info.NotAfter = cert.NotAfter.Format("2006-01-02 15:04:05")
	info.Days = int(time.Until(cert.NotAfter).Hours() / 24)
	info.Domains = cert.DNSNames
	info.Issuer = cert.Issuer.String()
	return info, true
}

// GetDomainCertificateInfo returns the per-domain certificate or the shared
// certificate marked as Shared when no dedicated certificate exists.
func GetDomainCertificateInfo(domain string) DomainCertificateInfo {
	keyPath, certPath := tlsutil.DomainCertPaths(domain)
	if info, ok := readCertificateInfo(keyPath, certPath); ok {
		result := DomainCertificateInfo{CertificateInfo: info, Domain: domain, Shared: false}
		if cfg, err := config.ReadConfig(); err == nil {
			if stored, exists := cfg.DomainCertificates[domain]; exists {
				result.Mode = stored.Mode
			}
		}
		return result
	}
	cfg, _ := config.ReadConfig()
	fallback := CertificateInfo{}
	if cfg != nil {
		if info, ok := readCertificateInfo(cfg.SSLPrivateKeyPath, cfg.SSLPublicKeyPath); ok {
			fallback = info
		}
		fallback.Type = cfg.SSLType
		fallback.Provider = cfg.DNSProvider
	}
	mode := ""
	if cfg != nil {
		if stored, exists := cfg.DomainCertificates[domain]; exists {
			mode = stored.Mode
		} else {
			switch cfg.SSLType {
			case config.SSLTypeAutoHTTP:
				mode = "http"
			case config.SSLTypeAutoDNS:
				if strings.EqualFold(cfg.DNSProvider, "cloudflare") {
					mode = "dns-cloudflare"
				} else {
					mode = "dns-manual"
				}
			case config.SSLTypeUser:
				mode = "upload"
			}
		}
	}
	return DomainCertificateInfo{CertificateInfo: fallback, Domain: domain, Shared: true, Mode: mode}
}

// IssueForDomain obtains a certificate that covers domain and its mail hosts.
func IssueForDomain(domain, mode, provider, token string) error {
	if GetStatus().State == StatusPending {
		return errors.New("another certificate request is already in progress")
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	known := false
	for _, configured := range cfg.Domains {
		if strings.EqualFold(configured, domain) {
			known = true
			break
		}
	}
	if !known {
		return errors.New("domain is not configured")
	}

	sslType := config.SSLTypeAutoHTTP
	switch mode {
	case "http":
		sslType = config.SSLTypeAutoHTTP
		provider = ""
		token = ""
	case "dns-manual":
		sslType = config.SSLTypeAutoDNS
		provider = "manual"
		token = ""
	case "dns-cloudflare":
		sslType = config.SSLTypeAutoDNS
		provider = "cloudflare"
		if strings.TrimSpace(token) == "" {
			return errors.New("Cloudflare API token is required")
		}
	default:
		return errors.New("unknown certificate mode")
	}

	if cfg.DomainCertificates == nil {
		cfg.DomainCertificates = map[string]config.DomainCertificate{}
	}
	cfg.DomainCertificates[domain] = config.DomainCertificate{Mode: mode, Provider: provider, Token: token}
	if err := config.WriteConfig(cfg); err != nil {
		return errors.Wrap(err)
	}

	keyPath, certPath := tlsutil.DomainCertPaths(domain)
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return errors.Wrap(err)
	}
	domains := []string{domain, "smtp." + domain, "imap." + domain, "pop." + domain}
	if cfg.WebDomain == domain || strings.HasSuffix(cfg.WebDomain, "."+domain) {
		domains = append(domains, cfg.WebDomain)
	}

	privateKey, newAccount := config.ReadPrivateKey()
	issuerPath := filepath.Join(filepath.Dir(certPath), "issuer.crt")
	go func() {
		if err := obtainCertificate(cfg, domains, sslType, provider, token, keyPath, certPath, issuerPath, newAccount, privateKey); err != nil {
			log.Errorf("domain %s certificate issuance failed: %+v", domain, err)
		}
	}()
	return nil
}

// RenewDueDomainCertificates issues a new certificate for the first automatic
// per-domain certificate that is missing or expires within 30 days. Uploaded and
// manual-DNS certificates are left to the operator.
func RenewDueDomainCertificates() {
	cfg, err := config.ReadConfig()
	if err != nil || cfg == nil || !cfg.IsInit {
		return
	}
	if GetStatus().State == StatusPending {
		return
	}
	for domain, certificate := range cfg.DomainCertificates {
		if certificate.Mode != "http" && certificate.Mode != "dns-cloudflare" {
			continue
		}
		info := GetDomainCertificateInfo(domain)
		if info.Configured && !info.Shared && info.Days >= 30 {
			continue
		}
		log.Infof("renewing certificate for domain %s", domain)
		if err := IssueForDomain(domain, certificate.Mode, certificate.Provider, certificate.Token); err != nil {
			log.Errorf("domain %s certificate renewal failed: %+v", domain, err)
		}
		return
	}
}

// UploadDomainCertificate installs an operator-provided certificate for a
// single domain.
func UploadDomainCertificate(domain, keyPEM, certPEM string) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	known := false
	for _, configured := range cfg.Domains {
		if strings.EqualFold(configured, domain) {
			known = true
			break
		}
	}
	if !known {
		return errors.New("domain is not configured")
	}
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		return errors.Wrap(err)
	}
	keyPath, certPath := tlsutil.DomainCertPaths(domain)
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return errors.Wrap(err)
	}
	if err := os.WriteFile(keyPath, []byte(keyPEM), 0600); err != nil {
		return errors.Wrap(err)
	}
	if err := os.WriteFile(certPath, []byte(certPEM), 0644); err != nil {
		return errors.Wrap(err)
	}
	if cfg.DomainCertificates == nil {
		cfg.DomainCertificates = map[string]config.DomainCertificate{}
	}
	cfg.DomainCertificates[domain] = config.DomainCertificate{Mode: "upload"}
	if err := config.WriteConfig(cfg); err != nil {
		return errors.Wrap(err)
	}
	setStatus(StatusSuccess, "Certificate uploaded.")
	return nil
}

// obtainCertificate runs ACME for an explicit name list and writes the result
// to the given paths. It is used by per-domain issuance from the settings UI.
func obtainCertificate(cfg *config.Config, domains []string, sslType, provider, token, keyPath, certPath, issuerPath string, newAccount bool, privateKey crypto.PrivateKey) error {
	setStatus(StatusPending, "Requesting certificate...")

	myUser := MyUser{Email: "i@" + cfg.Domain, key: privateKey}
	conf := lego.NewConfig(&myUser)
	conf.UserAgent = "iMail"
	conf.Certificate.KeyType = certcrypto.RSA2048
	client, err := lego.NewClient(conf)
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}

	if sslType == config.SSLTypeAutoHTTP {
		if err := client.Challenge.SetHTTP01Provider(GetHttpChallengeInstance()); err != nil {
			setStatus(StatusError, err.Error())
			return errors.Wrap(err)
		}
	} else if sslType == config.SSLTypeAutoDNS {
		if strings.EqualFold(provider, "cloudflare") && strings.TrimSpace(token) != "" {
			err = client.Challenge.SetDNS01Provider(NewCloudflareProvider(strings.TrimSpace(token)), dns01.AddDNSTimeout(10*time.Minute))
		} else {
			err = client.Challenge.SetDNS01Provider(GetDnsChallengeInstance(), dns01.AddDNSTimeout(60*time.Minute))
		}
		if err != nil {
			setStatus(StatusError, err.Error())
			return errors.Wrap(err)
		}
	} else {
		return errors.New("unsupported certificate mode")
	}

	var reg *registration.Resource
	if newAccount {
		reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	} else {
		reg, err = client.Registration.ResolveAccountByKey()
	}
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}
	myUser.Registration = reg

	request := certificate.ObtainRequest{Domains: domains, Bundle: true}
	certificates, obtainErr := client.Certificate.Obtain(request)
	if obtainErr != nil {
		setStatus(StatusError, obtainErr.Error())
		return errors.Wrap(obtainErr)
	}
	if writeErr := os.WriteFile(keyPath, certificates.PrivateKey, 0600); writeErr != nil {
		setStatus(StatusError, writeErr.Error())
		return errors.Wrap(writeErr)
	}
	if writeErr := os.WriteFile(certPath, certificates.Certificate, 0644); writeErr != nil {
		setStatus(StatusError, writeErr.Error())
		return errors.Wrap(writeErr)
	}
	_ = os.WriteFile(issuerPath, certificates.IssuerCertificate, 0644)
	setStatus(StatusSuccess, "Certificate issued.")
	time.Sleep(2 * time.Second)
	signal.RestartChan <- true
	return nil
}
