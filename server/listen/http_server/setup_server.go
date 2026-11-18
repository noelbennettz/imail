package http_server

import (
	"errors"
	"flag"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/controllers"
	"github.com/noelbennettz/imail/utils/ip"
	"github.com/noelbennettz/imail/utils/netutil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"
)

// Setup wizard server; exits once the setup wizard has finished
var setupServer *http.Server

func SetupStart() {
	mux := http.NewServeMux()
	fe, err := fs.Sub(local, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(fe)))
	mux.HandleFunc("/api/", contextIterceptor(controllers.Setup))
	// Challenge requests look like /.well-known/acme-challenge/QPyMAyaWw9s5JvV1oruyqWHG7OqkHMJEHPoUz2046KM
	mux.HandleFunc("/.well-known/", controllers.AcmeChallenge)

	HttpPort := 80
	flag.IntVar(&HttpPort, "p", 80, "HTTP port during the setup phase")
	flag.Parse()

	if HttpPort == 80 {
		HttpPort = cast.ToInt(os.Getenv("setup_port"))
	}

	if HttpPort <= 0 || HttpPort > 65535 {
		HttpPort = 80
	}

	config.Instance.SetSetupPort(HttpPort)
	log.Infof("HttpServer Start On Port :%d", HttpPort)
	if HttpPort == 80 {
		log.Infof("Please click http://%s to continue.\n", ip.GetIp())
	} else {
		log.Infof("Please click http://%s:%d to continue.", ip.GetIp(), HttpPort)
	}

	setupServer = &http.Server{
		Addr:         netutil.Address(config.Instance.ListenMode, HttpPort),
		Handler:      mux,
		ReadTimeout:  time.Second * 60,
		WriteTimeout: time.Second * 60,
	}
	listener, listenErr := net.Listen(netutil.Network(config.Instance.ListenMode),
		netutil.Address(config.Instance.ListenMode, HttpPort))
	if listenErr != nil {
		panic(listenErr)
	}
	err = setupServer.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func SetupStop() {
	err := setupServer.Close()
	log.Infof("Setup End!")
	if err != nil {
		panic(err)
	}
}
