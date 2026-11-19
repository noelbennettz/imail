package imap_server

import (
	"crypto/tls"
	"net"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/utils/netutil"
	"github.com/noelbennettz/imail/utils/tlsutil"
	log "github.com/sirupsen/logrus"
)

var (
	instanceTLS     *imapserver.Server
	serverTLSConfig *tls.Config
)

func Stop() {
	if instanceTLS != nil {
		instanceTLS.Close()
		instanceTLS = nil
	}
}

// StarTLS starts the implicit-TLS IMAP listener. Plaintext IMAP is not
// exposed; clients must negotiate TLS on 993.
func StarTLS() {
	instanceTLS = newIMAPServer()
	addr := netutil.Address(config.Instance.ListenMode, 993)
	listener, err := net.Listen(netutil.Network(config.Instance.ListenMode), addr)
	if err != nil {
		panic(err)
	}
	log.Infof("IMAP With TLS Server Start On %s", addr)
	if err := instanceTLS.Serve(tls.NewListener(listener, serverTLSConfig)); err != nil {
		panic(err)
	}
}

func newIMAPServer() *imapserver.Server {
	serverTLSConfig = tlsutil.ServerConfig()
	tlsConfig := serverTLSConfig

	memServer := NewServer()

	option := &imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.newSession(loginlimit.SourceKeyConn(conn.NetConn())), nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: {},
			imap.CapIdle:      {},
		},
		TLSConfig:    tlsConfig,
		InsecureAuth: false,
	}

	if config.Instance.LogLevel == "debug" {
		option.DebugWriter = os.Stdout
	}

	return imapserver.New(option)
}
