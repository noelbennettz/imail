package smtp_server

import (
	"crypto/tls"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/netutil"
	"github.com/noelbennettz/imail/utils/tlsutil"
	log "github.com/sirupsen/logrus"
)

var instance *smtp.Server
var instanceTls *smtp.Server
var instanceTlsNew *smtp.Server

// serverGreetingName is deliberately generic: the plaintext greeting must
// not disclose the configured mail domain.
const serverGreetingName = "imail"

func newSMTPServer(addr string) *smtp.Server {
	server := smtp.NewServer(&Backend{})
	server.Addr = addr
	server.Network = netutil.Network(config.Instance.ListenMode)
	server.Domain = serverGreetingName
	server.ReadTimeout = 10 * time.Second
	server.WriteTimeout = 10 * time.Second
	server.MaxMessageBytes = 1024 * 1024 * 30
	server.MaxRecipients = 50
	// PLAIN and LOGIN credentials may only be sent over TLS, including on 587.
	server.AllowInsecureAuth = false
	return server
}

func tlsConfig() *tls.Config {
	return tlsutil.ServerConfig()
}

func StartWithTLSNew() {
	instanceTlsNew = newSMTPServer(netutil.Address(config.Instance.ListenMode, 587))
	// 587 uses STARTTLS: plaintext greeting, upgraded handshake.
	instanceTlsNew.TLSConfig = tlsConfig()

	log.Println("Starting Smtp With STARTTLS Server Port:", instanceTlsNew.Addr)
	if err := instanceTlsNew.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func StartWithTLS() {
	instanceTls = newSMTPServer(netutil.Address(config.Instance.ListenMode, 465))
	instanceTls.TLSConfig = tlsConfig()

	log.Println("Starting Smtp With SSL Server Port:", instanceTls.Addr)
	if err := instanceTls.ListenAndServeTLS(); err != nil {
		log.Fatal(err)
	}
}

func Start() {
	instance = newSMTPServer(netutil.Address(config.Instance.ListenMode, 25))
	instance.TLSConfig = tlsConfig()

	log.Println("Starting Smtp Server Port:", instance.Addr)
	if err := instance.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func Stop() {
	if instance != nil {
		instance.Close()
	}
	if instanceTls != nil {
		instanceTls.Close()
	}

	if instanceTlsNew != nil {
		instanceTlsNew.Close()
	}
}
