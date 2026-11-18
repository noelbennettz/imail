package cron_server

import (
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/services/setup/ssl"
	"github.com/noelbennettz/imail/signal"
	log "github.com/sirupsen/logrus"
	"time"
	// Added: HTTP readiness probe
	"fmt"
	"net/http"
)

var expiredTime time.Time

func Start() {

	// On first start, wait until initialization completes
	if config.Instance == nil || config.IsInit == false {
		for {
			time.Sleep(1 * time.Minute)
			if config.Instance != nil && config.IsInit {
				break
			}
		}
	}

	autoDomainCertificate := false
	for _, certificate := range config.Instance.DomainCertificates {
		if certificate.Mode == "http" || certificate.Mode == "dns-cloudflare" {
			autoDomainCertificate = true
			break
		}
	}
	if config.Instance.SSLType == config.SSLTypeAutoHTTP || config.Instance.SSLType == config.SSLTypeAutoDNS || autoDomainCertificate {
		go sslUpdateLoop()
	} else {
		go sslCheck()
	}

}

// Check the SSL certificate for updates once a day and restart when it changed
func sslCheck() {
	var err error
	_, expiredTime, _, err = ssl.CheckSSLCrtInfo()
	if err != nil {
		panic(err)
	}

	for {
		time.Sleep(24 * time.Hour)
		_, newExpTime, _, err := ssl.CheckSSLCrtInfo()
		if err != nil {
			log.Errorf("SSL Check Error! %+v", err)
		}
		if newExpTime != expiredTime {
			expiredTime = newExpTime
			log.Infoln("SSL certificate had update! restarting")
			signal.RestartChan <- true
		}

	}
}

// Check once a day whether the SSL certificate is about to expire and regenerate it if so
func sslUpdateLoop() {
	for {
		// Wait for HTTP to be ready before running ACME renewal, so the challenge does not fail
		waitHTTPReady()
		ssl.Update(true)
		// Check certificate validity every 24 hours
		time.Sleep(24 * time.Hour)
	}
}

// Added: HTTP readiness probe (waits up to ~90 seconds)
func waitHTTPReady() {
	port := 80
	if config.Instance != nil && config.Instance.HttpPort > 0 {
		port = config.Instance.HttpPort
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/ping", port)

	for i := 0; i < 90; i++ {
		resp, err := http.Get(url)
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
	log.Warnf("HTTP not ready after 90s, skipping SSL update this round")
}
