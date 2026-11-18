package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/file"
	"github.com/noelbennettz/imail/utils/netutil"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var IsInit bool

type Config struct {
	LogLevel           string   `json:"logLevel"` // Log level
	Domain             string   `json:"domain"`
	Domains            []string `json:"domains"` // Multiple domains: list every domain that receives mail
	WebDomain          string   `json:"webDomain"`
	DkimPrivateKeyPath string   `json:"dkimPrivateKeyPath"`
	SSLType            string   `json:"sslType"` // 0 = auto-generated cert via HTTP challenge, 1 = user-uploaded cert, 2 = auto-generated cert via DNS challenge
	SSLPrivateKeyPath  string   `json:"SSLPrivateKeyPath"`
	SSLPublicKeyPath   string   `json:"SSLPublicKeyPath"`
	DbDSN              string   `json:"dbDSN"`
	DbType             string   `json:"dbType"`
	HttpsEnabled       int      `json:"httpsEnabled"`    // Whether the admin UI enforces https: 0 = default (enabled), 1 = enabled, 2 = disabled
	SpamFilterLevel    int      `json:"spamFilterLevel"` // Spam filtering level: 0 = off, 1 = filter when both spf and dkim fail, 2 = filter when spf fails, 3 = filter when dkim fails
	HttpPort           int      `json:"httpPort"`        // HTTP service port, default 80
	HttpsPort          int      `json:"httpsPort"`       // HTTPS service port, default 443
	IsInit             bool     `json:"isInit"`
	// ListenMode selects socket binding: both (dual stack), ipv4 or ipv6.
	ListenMode string `json:"listenMode"`
	// TLSMinVersion/TLSMaxVersion restrict negotiated protocol versions.
	TLSMinVersion string `json:"tlsMinVersion"`
	TLSMaxVersion string `json:"tlsMaxVersion"`
	// DisableSNICheck turns off strict SNI validation. Off by default:
	// connections without SNI or with an unknown name are refused.
	DisableSNICheck bool `json:"disableSNICheck"`
	// ServerHostnames adds extra names accepted during the TLS handshake.
	ServerHostnames []string `json:"serverHostnames"`
	// DNSProvider selects automatic DNS-01 validation ("cloudflare") or manual
	// record entry when empty. DNSApiToken is only used for automatic mode.
	DNSProvider string `json:"dnsProvider"`
	DNSApiToken string `json:"dnsApiToken"`
	// DomainCertificates stores per-domain TLS preferences. Certificates live
	// under config/ssl/domains/<domain>/.
	DomainCertificates map[string]DomainCertificate `json:"domainCertificates"`
	// DefaultQuotaMB applies to new users; 0 means unlimited.
	DefaultQuotaMB int               `json:"defaultQuotaMB"`
	Tables         map[string]string `json:"-"`
	TablesInitData map[string]string `json:"-"`
	setupPort      int               // Port used during the setup phase
}

var ROOT_PATH = ""

// DomainCertificate stores the per-domain TLS preferences.
type DomainCertificate struct {
	Mode     string `json:"mode"` // http, dns-manual, dns-cloudflare, upload
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

func init() {
	envs := os.Environ()
	for _, env := range envs {
		if strings.HasPrefix(env, "IMAIL_ROOT=") {
			ROOT_PATH = strings.TrimSpace(strings.ReplaceAll(env, "IMAIL_ROOT=", ""))
			if !strings.HasSuffix(ROOT_PATH, "/") {
				ROOT_PATH += "/"
			}

			fmt.Println("Env Root Path:", ROOT_PATH)
			return
		}
	}

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	realPath, err := filepath.EvalSymlinks(exPath)
	if err != nil {
		panic(err)
	}
	// Do not rewrite the root path when running inside GoLand
	if strings.Contains(realPath, "GoLand") && strings.Contains(realPath, "JetBrains") {
		return
	}

	if !strings.HasSuffix(realPath, "/") {
		realPath += "/"
	}
	ROOT_PATH = realPath
	fmt.Println("Root Path:", ROOT_PATH)
}

func (c *Config) GetSetupPort() int {
	return c.setupPort
}

func normalizeDomainName(domain string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(domain), ".")))
}

// ApplyDefaults normalizes optional settings loaded from JSON.
func (c *Config) ApplyDefaults() {
	c.ListenMode = netutil.NormalizeMode(c.ListenMode)
	if c.TLSMinVersion == "" {
		c.TLSMinVersion = "1.2"
	}
	if c.TLSMaxVersion == "" {
		c.TLSMaxVersion = "1.3"
	}

	c.Domain = normalizeDomainName(c.Domain)
	c.WebDomain = normalizeDomainName(c.WebDomain)
	if c.DomainCertificates == nil {
		c.DomainCertificates = map[string]DomainCertificate{}
	}
	seen := make(map[string]struct{}, len(c.Domains))
	domains := make([]string, 0, len(c.Domains))
	for _, domain := range c.Domains {
		domain = normalizeDomainName(domain)
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if c.Domain != "" {
		if _, exists := seen[c.Domain]; !exists {
			domains = append([]string{c.Domain}, domains...)
		}
	}
	c.Domains = domains
}

// TLSVersions returns the configured protocol bounds. The server only
// supports TLS 1.2 and TLS 1.3.
func (c *Config) TLSVersions() (uint16, uint16) {
	minVersion := netutil.TLSVersion(c.TLSMinVersion, tls.VersionTLS12)
	maxVersion := netutil.TLSVersion(c.TLSMaxVersion, tls.VersionTLS13)
	if maxVersion < minVersion {
		minVersion, maxVersion = maxVersion, minVersion
	}
	return minVersion, maxVersion
}

// TLSHostnames lists every name accepted during the TLS handshake.
func (c *Config) TLSHostnames() []string {
	seen := make(map[string]struct{})
	var hosts []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		hosts = append(hosts, name)
	}

	add(c.WebDomain)
	add(c.Domain)
	for _, domain := range c.Domains {
		add(domain)
		add("smtp." + domain)
		add("imap." + domain)
		add("pop." + domain)
	}
	for _, host := range c.ServerHostnames {
		add(host)
	}
	return hosts
}

// DefaultQuotaBytes converts the configured default quota to bytes.
func (c *Config) DefaultQuotaBytes() int64 {
	if c.DefaultQuotaMB <= 0 {
		return 0
	}
	return int64(c.DefaultQuotaMB) * 1024 * 1024
}

func (c *Config) SetSetupPort(setupPort int) {
	c.setupPort = setupPort
}

const DBTypeMySQL = "mysql"
const DBTypeSQLite = "sqlite"
const DBTypePostgres = "postgres"
const SSLTypeAutoHTTP = "0" // Auto-generated certificate
const SSLTypeAutoDNS = "2"  // Auto-generated certificate validated via DNS API
const SSLTypeUser = "1"     // User-uploaded certificate

var DBTypes []string = []string{DBTypeMySQL, DBTypeSQLite, DBTypePostgres}

var Instance *Config = &Config{}

type logFormatter struct {
}

// Format defines the log output format
func (l *logFormatter) Format(entry *log.Entry) ([]byte, error) {
	b := bytes.Buffer{}

	b.WriteString(fmt.Sprintf("[%s]", entry.Level.String()))
	b.WriteString(fmt.Sprintf("[%s]", entry.Time.Format("2006-01-02 15:04:05")))
	if entry.Context != nil {
		ctx := entry.Context.(*context.Context)
		if ctx != nil {
			b.WriteString(fmt.Sprintf("[%s]", ctx.GetValue(context.LogID)))
		}
	}
	b.WriteString(fmt.Sprintf("[%s:%d]", entry.Caller.File, entry.Caller.Line))
	b.WriteString(entry.Message)

	b.WriteString("\n")
	return b.Bytes(), nil
}
func Init() {
	var cfgData []byte
	var err error
	args := os.Args

	if len(args) >= 2 && args[len(args)-1] == "dev" {
		cfgData, err = os.ReadFile(ROOT_PATH + "./config/config.dev.json")
		if err != nil {
			return
		}
	} else {
		cfgData, err = os.ReadFile(ROOT_PATH + "./config/config.json")
		if err != nil {
			log.Errorf("config file not found,%s", err.Error())
			return
		}
	}

	err = json.Unmarshal(cfgData, &Instance)
	Instance.ApplyDefaults()
	Instance.fixPath()
	if err != nil {
		return
	}

	if len(Instance.Domains) == 0 && Instance.Domain != "" {
		Instance.Domains = []string{Instance.Domain}
	}

	if Instance.Domain != "" && Instance.IsInit {
		IsInit = true
	}

	// Use the custom log formatter
	log.SetFormatter(&logFormatter{})
	log.SetReportCaller(true)

	// Send logs to stdout (the default output is stderr)
	// Any io.Writer can be used as the log output
	log.SetOutput(os.Stdout)

	// All persisted timestamps are UTC. The web client renders them in the
	// device timezone; do not pin the process to a server-local zone.
	time.Local = time.UTC
	if Instance != nil {
		switch Instance.LogLevel {
		case "":
			log.SetLevel(log.InfoLevel)
		case "debug":
			log.SetLevel(log.DebugLevel)
		case "info":
			log.SetLevel(log.InfoLevel)
		case "warn":
			log.SetLevel(log.WarnLevel)
		case "error":
			log.SetLevel(log.ErrorLevel)
		default:
			log.SetLevel(log.InfoLevel)
		}
	} else {
		log.SetLevel(log.InfoLevel)
	}

}

func ReadPrivateKey() (*ecdsa.PrivateKey, bool) {
	key, err := os.ReadFile(ROOT_PATH + "./config/ssl/account_private.pem")
	if err != nil {
		return createNewPrivateKey(), true
	}

	block, _ := pem.Decode(key)
	x509Encoded := block.Bytes
	privateKey, _ := x509.ParseECPrivateKey(x509Encoded)

	return privateKey, false
}

func createNewPrivateKey() *ecdsa.PrivateKey {
	// Create a user. New accounts need an email and private key to start.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	x509Encoded, _ := x509.MarshalECPrivateKey(privateKey)

	// Write the EC key to a PEM file
	keypem, _ := os.OpenFile(ROOT_PATH+"./config/ssl/account_private.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	err = pem.Encode(keypem, &pem.Block{Type: "EC PRIVATE KEY", Bytes: x509Encoded})
	if err != nil {
		panic(err)
	}
	return privateKey
}

var configWriteMu sync.Mutex

func WriteConfig(cfg *Config) error {
	bytes, err := json.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err)
	}
	configWriteMu.Lock()
	defer configWriteMu.Unlock()
	if err := os.MkdirAll(ROOT_PATH+"/config/", 0700); err != nil {
		return errors.Wrap(err)
	}
	path := ROOT_PATH + "./config/config.json"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return errors.Wrap(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return errors.Wrap(err)
	}
	return nil
}

func ReadConfig() (*Config, error) {
	configData := Config{
		DkimPrivateKeyPath: ROOT_PATH + "config/dkim/dkim.priv",
		SSLPrivateKeyPath:  ROOT_PATH + "config/ssl/private.key",
		SSLPublicKeyPath:   ROOT_PATH + "config/ssl/public.crt",
	}
	if !file.PathExist(ROOT_PATH + "./config/config.json") {
		bytes, _ := json.Marshal(configData)
		_ = os.MkdirAll(ROOT_PATH+"/config/", 0700)
		err := os.WriteFile(ROOT_PATH+"./config/config.json", bytes, 0600)
		if err != nil {
			log.Errorf("Write Config Error:%s", err.Error())
			return nil, errors.Wrap(err)
		}
	} else {
		cfgData, err := os.ReadFile(ROOT_PATH + "./config/config.json")
		if err != nil {
			log.Errorf("Read Config Error:%s", err.Error())
			return nil, errors.Wrap(err)
		}
		_ = os.Chmod(ROOT_PATH+"./config/config.json", 0600)

		err = json.Unmarshal(cfgData, &configData)
		configData.ApplyDefaults()
		configData.fixPath()
		if err != nil {
			log.Errorf("Read Config Unmarshal Error:%s", err.Error())
			return nil, errors.Wrap(err)
		}
	}
	return &configData, nil
}

func (c *Config) fixPath() {
	if c.DbType == DBTypeSQLite && !strings.HasPrefix(c.DbDSN, "/") {
		c.DbDSN = ROOT_PATH + c.DbDSN
	}
	if !strings.HasPrefix(c.SSLPublicKeyPath, "/") {
		c.SSLPublicKeyPath = ROOT_PATH + c.SSLPublicKeyPath
	}
	if !strings.HasPrefix(c.SSLPrivateKeyPath, "/") {
		c.SSLPrivateKeyPath = ROOT_PATH + c.SSLPrivateKeyPath
	}
	if c.DkimPrivateKeyPath != "" && !strings.HasPrefix(c.DkimPrivateKeyPath, "/") {
		c.DkimPrivateKeyPath = ROOT_PATH + c.DkimPrivateKeyPath
	}
}

// Redacted returns a copy safe to include in logs: secrets are removed.
func (c *Config) Redacted() *Config {
	copyCfg := *c
	copyCfg.DNSApiToken = ""
	copyCfg.DomainCertificates = make(map[string]DomainCertificate, len(c.DomainCertificates))
	for domain, certificate := range c.DomainCertificates {
		certificate.Token = ""
		copyCfg.DomainCertificates[domain] = certificate
	}
	return &copyCfg
}
