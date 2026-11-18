package ssl

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/services/setup"
	"github.com/noelbennettz/imail/signal"
	"github.com/noelbennettz/imail/utils/async"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

type MyUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// Status reports the progress of automatic certificate issuance so the setup
// UI can show manual DNS instructions and real errors instead of hanging.
type Status struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

const (
	StatusIdle    = "idle"
	StatusPending = "pending"
	StatusSuccess = "success"
	StatusError   = "error"
)

var (
	statusMu sync.Mutex
	status   = Status{State: StatusIdle}
)

func GetStatus() Status {
	statusMu.Lock()
	defer statusMu.Unlock()
	return status
}

func setStatus(state, message string) {
	statusMu.Lock()
	status = Status{State: state, Message: message}
	statusMu.Unlock()
}

func (u *MyUser) GetEmail() string {
	return u.Email
}

func (u MyUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *MyUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func GetSSL() string {
	cfg, err := config.ReadConfig()
	if err != nil {
		panic(err)
	}
	if cfg.SSLType == "" {
		return config.SSLTypeAutoHTTP
	}

	return cfg.SSLType
}

func SetSSL(sslType, priKey, crtKey string) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		panic(err)
	}
	if sslType == config.SSLTypeAutoHTTP || sslType == config.SSLTypeUser || sslType == config.SSLTypeAutoDNS {
		cfg.SSLType = sslType
	} else {
		return errors.New("SSL Type Error!")
	}

	if cfg.SSLType != config.SSLTypeUser && strings.TrimSpace(cfg.Domain) == "" {
		return errors.New("Domain must be configured before automatic SSL certificate setup")
	}

	if cfg.SSLType == config.SSLTypeUser {
		cfg.SSLPrivateKeyPath = priKey
		cfg.SSLPublicKeyPath = crtKey
		// With a manually configured certificate the admin UI does not enable https by default
		cfg.HttpsEnabled = 2
	}

	err = config.WriteConfig(cfg)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func renewCertificate(privateKey *ecdsa.PrivateKey, cfg *config.Config) error {
	setStatus(StatusPending, "Renewing certificate...")

	myUser := MyUser{
		Email: "SSLMaster@" + cfg.Domain,
		key:   privateKey,
	}

	conf := lego.NewConfig(&myUser)
	conf.UserAgent = "iMail"
	conf.Certificate.KeyType = certcrypto.RSA2048

	// A client facilitates communication with the CA server.
	client, err := lego.NewClient(conf)
	if err != nil {
		return errors.Wrap(err)
	}

	if cfg.SSLType == config.SSLTypeAutoHTTP {
		err = client.Challenge.SetHTTP01Provider(GetHttpChallengeInstance())
		if err != nil {
			return errors.Wrap(err)
		}
	} else if cfg.SSLType == config.SSLTypeAutoDNS {
		err = client.Challenge.SetDNS01Provider(GetDnsChallengeInstance(), dns01.AddDNSTimeout(60*time.Minute))
		if err != nil {
			return errors.Wrap(err)
		}

		log.Errorf("Please Set DNS Record/Add the following entries to your DNS records:\n")
		for _, item := range GetDnsChallengeInstance().GetDNSSettings(nil) {
			log.Errorf("Type:%s\tHost:%s\tValue:%s\n", item.Type, item.Host, item.Value)
		}

	}

	var reg *registration.Resource

	reg, err = client.Registration.ResolveAccountByKey()
	if err != nil {
		return errors.Wrap(err)
	}

	myUser.Registration = reg

	domains := certificateDomains(cfg)

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	log.Infof("wait ssl renew")
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}
	err = os.WriteFile(cfg.SSLPrivateKeyPath, certificates.PrivateKey, 0o600)
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}

	err = os.WriteFile(cfg.SSLPublicKeyPath, certificates.Certificate, 0o644)
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}

	err = os.WriteFile(filepath.Join(filepath.Dir(cfg.SSLPublicKeyPath), "issuerCert.crt"), certificates.IssuerCertificate, 0o644)
	if err != nil {
		setStatus(StatusError, err.Error())
		return errors.Wrap(err)
	}
	setStatus(StatusSuccess, "Certificate issued.")

	return nil
}

// certificateDomains returns a de-duplicated certificate name list and skips
// empty/invalid entries so ACME never receives identifiers such as "imap.".
func certificateDomains(cfg *config.Config) []string {
	seen := map[string]bool{}
	var domains []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), ".")))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		domains = append(domains, name)
	}
	add(cfg.WebDomain)
	for _, domain := range cfg.Domains {
		domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(domain), ".")))
		if domain == "" {
			continue
		}
		add("smtp." + domain)
		add("pop." + domain)
		add("imap." + domain)
	}
	return domains
}

func generateCertificate(privateKey *ecdsa.PrivateKey, cfg *config.Config, newAccount bool, finishSetup bool) error {
	if strings.TrimSpace(cfg.Domain) == "" {
		return errors.New("Domain must be configured before automatic SSL certificate setup")
	}
	setStatus(StatusPending, "Requesting certificate...")

	myUser := MyUser{
		Email: "i@" + cfg.Domain,
		key:   privateKey,
	}

	conf := lego.NewConfig(&myUser)
	conf.UserAgent = "iMail"
	conf.Certificate.KeyType = certcrypto.RSA2048

	// A client facilitates communication with the CA server.
	client, err := lego.NewClient(conf)
	if err != nil {
		return errors.Wrap(err)
	}

	if cfg.SSLType == config.SSLTypeAutoHTTP {
		err = client.Challenge.SetHTTP01Provider(GetHttpChallengeInstance())
		if err != nil {
			return errors.Wrap(err)
		}
	} else if cfg.SSLType == config.SSLTypeAutoDNS {
		if strings.EqualFold(cfg.DNSProvider, "cloudflare") && strings.TrimSpace(cfg.DNSApiToken) != "" {
			err = client.Challenge.SetDNS01Provider(NewCloudflareProvider(strings.TrimSpace(cfg.DNSApiToken)), dns01.AddDNSTimeout(10*time.Minute))
		} else {
			err = client.Challenge.SetDNS01Provider(GetDnsChallengeInstance(), dns01.AddDNSTimeout(60*time.Minute))
		}
		if err != nil {
			return errors.Wrap(err)
		}
	}

	var reg *registration.Resource

	if newAccount {
		reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return errors.Wrap(err)
		}
	} else {
		reg, err = client.Registration.ResolveAccountByKey()
		if err != nil {
			return errors.Wrap(err)
		}
	}

	myUser.Registration = reg

	domains := certificateDomains(cfg)

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	as := async.New(nil)

	as.Process(func(params any) {
		log.Infof("wait ssl")
		certificates, err := client.Certificate.Obtain(request)
		if err != nil {
			log.Errorf("certificate obtain failed: %v", err)
			setStatus(StatusError, err.Error())
			return
		}
		log.Infof("Certificate verification passed!")
		err = os.WriteFile(cfg.SSLPrivateKeyPath, certificates.PrivateKey, 0o600)
		if err != nil {
			setStatus(StatusError, err.Error())
			return
		}

		err = os.WriteFile(cfg.SSLPublicKeyPath, certificates.Certificate, 0o644)
		if err != nil {
			setStatus(StatusError, err.Error())
			return
		}

		err = os.WriteFile(filepath.Join(filepath.Dir(cfg.SSLPublicKeyPath), "issuerCert.crt"), certificates.IssuerCertificate, 0o644)
		if err != nil {
			setStatus(StatusError, err.Error())
			return
		}

		setStatus(StatusSuccess, "Certificate issued.")
		if finishSetup {
			setup.Finish()
		} else {
			go func() {
				time.Sleep(2 * time.Second)
				signal.RestartChan <- true
			}()
		}
	}, nil)

	return nil
}

func GenSSL(update bool) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		panic(err)
	}

	if !update {
		privateFile, errpi := os.ReadFile(cfg.SSLPrivateKeyPath)
		public, errpu := os.ReadFile(cfg.SSLPublicKeyPath)
		// Certificate data already exists, so do not generate one
		if errpi == nil && errpu == nil && len(privateFile) > 0 && len(public) > 0 {
			return nil
		}
	}

	privateKey, newAccount := config.ReadPrivateKey()

	if !update {
		err = generateCertificate(privateKey, cfg, newAccount, true)
		if err != nil {
			setStatus(StatusError, err.Error())
		}
		return err
	}

	err = renewCertificate(privateKey, cfg)
	if err != nil {
		setStatus(StatusError, err.Error())
	}
	return err
}

// IssueNow requests a certificate for every configured domain. Unlike the
// setup path it never waits for the setup channel; listeners are restarted
// automatically after the certificate is written.
func IssueNow() error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	privateKey, newAccount := config.ReadPrivateKey()
	return generateCertificate(privateKey, cfg, newAccount, false)
}

// CertificateInfo describes the certificate currently on disk.
type CertificateInfo struct {
	Configured bool     `json:"configured"`
	Type       string   `json:"type"`
	Provider   string   `json:"provider"`
	NotAfter   string   `json:"not_after"`
	Days       int      `json:"days"`
	Domains    []string `json:"domains"`
	Issuer     string   `json:"issuer"`
	Error      string   `json:"error"`
}

func GetCertificateInfo() CertificateInfo {
	cfg, err := config.ReadConfig()
	if err != nil {
		return CertificateInfo{Error: err.Error()}
	}
	info := CertificateInfo{Type: cfg.SSLType, Provider: cfg.DNSProvider}
	tlsCert, err := tls.LoadX509KeyPair(cfg.SSLPublicKeyPath, cfg.SSLPrivateKeyPath)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Configured = true
	info.NotAfter = cert.NotAfter.Format("2006-01-02 15:04:05")
	info.Days = cast.ToInt(time.Until(cert.NotAfter).Hours() / 24)
	info.Domains = cert.DNSNames
	info.Issuer = cert.Issuer.String()
	return info
}

// UploadCertificate installs an operator-provided key/certificate pair.
func UploadCertificate(keyPEM, certPEM string) error {
	if strings.TrimSpace(keyPEM) == "" || strings.TrimSpace(certPEM) == "" {
		return errors.New("key and certificate are required")
	}
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		return errors.Wrap(err)
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	if err := os.WriteFile(cfg.SSLPrivateKeyPath, []byte(keyPEM), 0o600); err != nil {
		return errors.Wrap(err)
	}
	if err := os.WriteFile(cfg.SSLPublicKeyPath, []byte(certPEM), 0o644); err != nil {
		return errors.Wrap(err)
	}
	cfg.SSLType = config.SSLTypeUser
	if err := config.WriteConfig(cfg); err != nil {
		return errors.Wrap(err)
	}
	config.Instance.SSLType = cfg.SSLType
	setStatus(StatusSuccess, "Certificate uploaded.")
	return nil
}

func CheckSSLCrtInfo() (int, time.Time, bool, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		panic(err)
	}
	// load cert and key by tls.LoadX509KeyPair
	tlsCert, err := tls.LoadX509KeyPair(cfg.SSLPublicKeyPath, cfg.SSLPrivateKeyPath)
	if err != nil {
		return -1, time.Now(), true, errors.Wrap(err)
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return -1, time.Now(), true, errors.Wrap(err)
	}

	nameMatchFail := true
	for _, name := range cert.DNSNames {
		if strings.Contains(name, "imap") {
			nameMatchFail = false
			break
		}
	}

	// Check the expiration time
	hours := cert.NotAfter.Sub(time.Now()).Hours()

	if hours <= 0 {
		return -1, time.Now(), nameMatchFail, errors.New("Certificate has expired")
	}

	return cast.ToInt(hours / 24), cert.NotAfter, nameMatchFail, nil
}

func Update(needRestart bool) {
	if config.Instance != nil && config.Instance.IsInit {
		RenewDueDomainCertificates()
	}
	if config.Instance != nil && config.Instance.IsInit && (config.Instance.SSLType == config.SSLTypeAutoHTTP || config.Instance.SSLType == config.SSLTypeAutoDNS) {
		days, _, nameMatchFail, err := CheckSSLCrtInfo()

		if days < 30 || err != nil || nameMatchFail {
			if err != nil {
				log.Errorf("SSL Check Error, Update SSL Certificate. Error Info :%+v", err)
			} else {
				log.Infof("SSL certificate remaining time is only %d days, renew SSL certificate.", days)
			}
			err = GenSSL(true)
			if err != nil {
				log.Errorf("SSL Update Error! %+v", err)
			}
			if needRestart {
				// Restart the service after updating the certificate
				signal.RestartChan <- true
			}
		} else {
			log.Debugf("SSL Check.")
		}
	}
}
