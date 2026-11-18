package pop3_server

import (
	"crypto/rand"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/third_party/gopop"
	"github.com/noelbennettz/imail/utils/netutil"
	"github.com/noelbennettz/imail/utils/tlsutil"
	log "github.com/sirupsen/logrus"
)

var (
	instance    *gopop.Server
	instanceTls *gopop.Server
)

// serverGreetingName is deliberately generic: the POP3 greeting must not
// disclose the configured mail domain.
const serverGreetingName = "imail"

func buildServer(port int, implicitTLS bool) *gopop.Server {
	tlsConfig := tlsutil.ServerConfig()
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader

	server := gopop.NewPop3Server(port, serverGreetingName, implicitTLS, tlsConfig, action{})
	server.Addr = netutil.Address(config.Instance.ListenMode, port)
	server.Network = netutil.Network(config.Instance.ListenMode)
	server.ConnectAliveTime = 5 * time.Minute
	return server
}

func StartWithTls() {
	instanceTls = buildServer(995, true)
	log.Infof("POP3 With TLS Server Start On %s", instanceTls.Addr)

	if err := instanceTls.Start(); err != nil {
		panic(err)
	}
}

func Start() {
	instance = buildServer(110, false)
	log.Infof("POP3 Server Start On %s", instance.Addr)

	if err := instance.Start(); err != nil {
		panic(err)
	}
}

func Stop() {
	if instance != nil {
		instance.Stop()
	}

	if instanceTls != nil {
		instanceTls.Stop()
	}
}
