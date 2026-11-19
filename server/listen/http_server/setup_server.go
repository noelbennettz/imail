package http_server

import (
	"errors"
	"flag"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/controllers"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/setup"
	"github.com/noelbennettz/imail/utils/ip"
	"github.com/noelbennettz/imail/utils/netutil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

// Paths of the setup wizard API. Everything except these, the static client,
// /api/ping and the ACME challenge needs the one-time setup token.
const (
	setupAPIPath       = "/api/setup"
	setupStatusPath    = "/api/setup/status"
	setupVerifyPath    = "/api/setup/token/verify"
	pingPath           = "/api/ping"
	acmeChallengePath  = "/.well-known/"
	setupTokenFailBody = `{"errorNo":406,"errorMsg":"setup token required","data":null}`
)

// Setup wizard server; exits once the setup wizard has finished
var setupServer *http.Server

// newSetupMux builds the HTTP handler of the setup wizard. It is separate from
// SetupStart so tests can exercise it without binding a port.
func newSetupMux(tokens *setup.SetupToken) http.Handler {
	mux := http.NewServeMux()
	fe, err := fs.Sub(local, "dist")
	if err != nil {
		panic(err)
	}
	// Static client assets and the API paths that must stay reachable without a
	// token: the health check and the ACME HTTP-01 challenge.
	mux.Handle("/", http.FileServer(http.FS(fe)))
	mux.HandleFunc(acmeChallengePath, controllers.AcmeChallenge)
	mux.HandleFunc(pingPath, controllers.Ping)

	// Minimal status for the first screen: whether setup is still open and
	// whether a token is required. Nothing else is disclosed.
	mux.HandleFunc(setupStatusPath, func(w http.ResponseWriter, r *http.Request) {
		response.NewSuccessResponse(map[string]bool{
			"initialized":    config.IsInit,
			"requires_token": tokens.Enabled(),
		}).FPrint(w)
	})

	// The first screen asks the browser to prove that it holds the token.
	mux.HandleFunc(setupVerifyPath, requireSetupToken(tokens, func(w http.ResponseWriter, r *http.Request) {
		response.NewSuccessResponse(map[string]bool{"ok": true}).FPrint(w)
	}))

	mux.HandleFunc(setupAPIPath, requireSetupToken(tokens, contextIterceptor(controllers.Setup)))

	// No other API path exists during setup.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		response.NewErrorResponse(response.ParamsError, "unknown setup endpoint", "").FPrint(w)
	})
	return mux
}

// requireSetupToken rejects every request without a valid setup token. Missing,
// wrong and closed all get the same 403 body; a source address that failed too
// often gets 429 instead.
func requireSetupToken(tokens *setup.SetupToken, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := tokens.Verify(r.RemoteAddr, r.Header.Get(setup.TokenHeader))
		if result != setup.VerifyAllowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(result.HTTPStatus())
			w.Write([]byte(setupTokenFailBody))
			return
		}
		next(w, r)
	}
}

func SetupStart() {
	tokens, err := setup.InitToken(config.IsInit)
	if err != nil {
		panic(err)
	}
	mux := newSetupMux(tokens)

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
