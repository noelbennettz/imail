package res_init

import (
	"encoding/json"
	"os"
	"time"

	"fmt"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/listen/http_server"
	"github.com/noelbennettz/imail/listen/imap_server"
	"github.com/noelbennettz/imail/listen/pop3_server"
	"github.com/noelbennettz/imail/listen/smtp_server"
	"github.com/noelbennettz/imail/services/setup/ssl"
	"github.com/noelbennettz/imail/session"
	"github.com/noelbennettz/imail/signal"
	"github.com/noelbennettz/imail/utils/file"
	"github.com/noelbennettz/imail/utils/netutil"
	log "github.com/sirupsen/logrus"
	// Added: HTTP readiness probe
	"net/http"
)

func Init(serverVersion string) {

	if !config.IsInit {
		dirInit()

		go http_server.SetupStart()
		<-signal.InitChan
		http_server.SetupStop()
	}

	for {
		config.Init()
		// Removed: immediate certificate update before startup, which would
		// trigger ACME while HTTP is not yet listening
		// ssl.Update(false)

		parsemail.Init()
		err := db.Init(serverVersion)
		if err != nil {
			panic(err)
		}
		session.Init()
		// smtp server start
		go smtp_server.Start()
		go smtp_server.StartWithTLS()
		go smtp_server.StartWithTLSNew()
		// http server start
		go http_server.HttpsStart()
		go http_server.HttpStart()

		// Wait until the HTTP service is ready before checking/renewing
		// certificates so ACME validation does not fail
		waitHTTPReady()

		// Run one certificate check after startup (non-restarting mode)
		ssl.Update(false)

		// pop3 server start
		go pop3_server.Start()
		go pop3_server.StartWithTls()
		// imap server start
		go imap_server.StarTLS()

		configStr, _ := json.Marshal(config.Instance.Redacted())
		log.Warnf("Config File Info:  %s", configStr)

		select {
		case <-signal.RestartChan:
			log.Infof("Server Restart!")
			smtp_server.Stop()
			http_server.HttpsStop()
			http_server.HttpStop()
			pop3_server.Stop()
			imap_server.Stop()
		case <-signal.StopChan:
			log.Infof("Server Stop!")
			smtp_server.Stop()
			http_server.HttpsStop()
			http_server.HttpStop()
			pop3_server.Stop()
			imap_server.Stop()
			return
		}
		log.Infof("Server Stop Success!")
		time.Sleep(5 * time.Second)

	}

}

func dirInit() {
	if !file.PathExist("./config") {
		err := os.MkdirAll("./config", 0744)
		if err != nil {
			panic(err)
		}
	}

	if !file.PathExist("./config/dkim") {
		err := os.MkdirAll("./config/dkim", 0744)
		if err != nil {
			panic(err)
		}
	}

	if !file.PathExist("./config/ssl") {
		err := os.MkdirAll("./config/ssl", 0744)
		if err != nil {
			panic(err)
		}
	}
}

// waitHTTPReady probes the local HTTP endpoint (up to ~90 seconds) before
// starting certificate renewal, so the ACME HTTP challenge can be served.
func waitHTTPReady() {
	port := 80
	if config.Instance != nil && config.Instance.HttpPort > 0 {
		port = config.Instance.HttpPort
	}
	url := fmt.Sprintf("http://%s/api/ping", netutil.LoopbackAddress(config.Instance.ListenMode, port))

	for i := 0; i < 90; i++ {
		resp, err := netutil.ProbeClient.Get(url)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			log.Infof("HTTP ready: %s", url)
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	log.Warnf("HTTP not ready after 90s, skipping immediate SSL update")
}
