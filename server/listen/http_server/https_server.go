package http_server

import (
	"embed"
	"errors"
	olog "log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/controllers"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/session"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/id"
	"github.com/noelbennettz/imail/utils/netutil"
	"github.com/noelbennettz/imail/utils/tlsutil"

	log "github.com/sirupsen/logrus"
)

// The front end now emits chunks whose names start with an underscore (Vite's
// shared helpers). A bare dist/* pattern skips those silently, which left the
// binary serving an index.html full of 404s; all:dist includes every file.
//
//go:embed all:dist
var local embed.FS

var httpsServer *http.Server

type nullWrite struct{}

func (w *nullWrite) Write(p []byte) (int, error) {
	return len(p), nil
}

func HttpsStart() {
	mux := http.NewServeMux()

	router(mux)

	// The go http server prints a lot of useless logs; use an empty log handler to suppress them
	nullLog := olog.New(&nullWrite{}, "", olog.Ldate)

	HttpsPort := 443
	if config.Instance.HttpsPort > 0 {
		HttpsPort = config.Instance.HttpsPort
	}

	if config.Instance.HttpsEnabled != 2 {
		tlsConfig := tlsutil.ServerConfig()
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}

		log.Infof("Https Server Start On %s", netutil.Address(config.Instance.ListenMode, HttpsPort))
		httpsServer = &http.Server{
			Addr:         netutil.Address(config.Instance.ListenMode, HttpsPort),
			Handler:      session.Instance.LoadAndSave(mux),
			ReadTimeout:  time.Second * 90,
			WriteTimeout: time.Second * 90,
			ErrorLog:     nullLog,
			TLSConfig:    tlsConfig,
		}
		listener, listenErr := net.Listen(netutil.Network(config.Instance.ListenMode),
			netutil.Address(config.Instance.ListenMode, HttpsPort))
		if listenErr != nil {
			panic(listenErr)
		}
		var err error
		err = httpsServer.ServeTLS(listener, "", "")
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				// Normal shutdown (restart or stop)
				log.Infof("Https Server closed normally on port :%d", HttpsPort)
			} else {
				// Unexpected errors are still logged and then exit
				log.Errorf("Https Server error on port :%d, %+v", HttpsPort, err)
				panic(err)
			}
		}
	}
}

func HttpsStop() {
	if httpsServer != nil {
		httpsServer.Close()
	}
}

// Added: handle close errors separately

// Injects the context
func contextIterceptor(h controllers.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if config.Instance != nil && config.Instance.HttpsEnabled != 2 {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, parseErr := url.Parse(origin)
				if parseErr != nil || !strings.EqualFold(parsed.Host, r.Host) {
					response.NewErrorResponse(response.NoAccessPrivileges, "cross-origin request rejected", "").FPrint(w)
					return
				}
			}
		}
		// Bound request bodies for all API endpoints (attachments included).
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

		ctx := &context.Context{}
		ctx.Context = r.Context()
		ctx.SetValue(context.LogID, id.GenLogID())
		lang := r.Header.Get("Lang")
		if lang == "" {
			lang = "en"
		}
		ctx.Lang = lang

		if config.IsInit {
			userInfo, err := session.CurrentUser(ctx)
			if err != nil {
				log.WithContext(ctx).Errorf("Validate session: %v", err)
				response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
				return
			}
			if userInfo != nil {
				ctx.UserID = userInfo.ID
				ctx.UserName = userInfo.Name
				ctx.UserAccount = userInfo.Account
				ctx.UserDomain = userInfo.Domain
				ctx.IsAdmin = userInfo.IsAdmin == 1
			}

			if ctx.UserID == 0 {
				if r.URL.Path != "/api/ping" && r.URL.Path != "/api/login" {
					response.NewErrorResponse(response.NeedLogin, i18n.GetText(ctx.Lang, "login_exp"), "").FPrint(w)
					return
				}
			}
		} else if r.URL.Path != "/api/setup" {
			response.NewErrorResponse(response.NeedSetup, "", "").FPrint(w)
			return
		}
		h(ctx, w, r)
	}
}
