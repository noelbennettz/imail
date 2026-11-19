package http_server

import (
	"errors"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/controllers"
	"github.com/noelbennettz/imail/controllers/email"
	"github.com/noelbennettz/imail/session"
	"github.com/noelbennettz/imail/utils/netutil"
	log "github.com/sirupsen/logrus"
)

// This server intercepts http requests and redirects them to https
var httpServer *http.Server

func HttpStop() {
	if httpServer != nil {
		httpServer.Close()
	}
}

func router(mux *http.ServeMux) {
	fe, err := fs.Sub(local, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", securityHeaders(http.FileServer(http.FS(fe))))
	// Challenge requests look like /.well-known/acme-challenge/QPyMAyaWw9s5JvV1oruyqWHG7OqkHMJEHPoUz2046KM
	mux.HandleFunc("/.well-known/", controllers.AcmeChallenge)
	mux.HandleFunc("/api/ping", controllers.Ping)
	mux.HandleFunc("/api/login", contextIterceptor(controllers.Login))
	mux.HandleFunc("/api/logout", contextIterceptor(controllers.Logout))
	mux.HandleFunc("/api/group", contextIterceptor(controllers.GetUserGroup))
	mux.HandleFunc("/api/group/list", contextIterceptor(controllers.GetUserGroupList))
	mux.HandleFunc("/api/group/add", contextIterceptor(controllers.AddGroup))
	mux.HandleFunc("/api/group/del", contextIterceptor(controllers.DelGroup))
	mux.HandleFunc("/api/email/list", contextIterceptor(email.EmailList))
	mux.HandleFunc("/api/email/del", contextIterceptor(email.EmailDelete))
	mux.HandleFunc("/api/email/read", contextIterceptor(email.MarkRead))
	mux.HandleFunc("/api/email/detail", contextIterceptor(email.EmailDetail))
	mux.HandleFunc("/api/email/thread", contextIterceptor(email.EmailThread))
	mux.HandleFunc("/api/email/source", contextIterceptor(email.EmailSource))
	mux.HandleFunc("/api/email/move", contextIterceptor(email.Move))
	mux.HandleFunc("/api/email/send", contextIterceptor(email.Send))
	mux.HandleFunc("/api/settings/modify_password", contextIterceptor(controllers.ModifyPassword))
	mux.HandleFunc("/api/admin/security", contextIterceptor(controllers.SecuritySettings))
	mux.HandleFunc("/api/admin/security/save", contextIterceptor(controllers.SaveSecuritySettings))
	mux.HandleFunc("/api/admin/outbox", contextIterceptor(controllers.OutboxStatus))
	mux.HandleFunc("/api/rule/get", contextIterceptor(controllers.GetRule))
	mux.HandleFunc("/api/rule/add", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/update", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/del", contextIterceptor(controllers.DelRule))
	mux.HandleFunc("/attachments/", contextIterceptor(controllers.GetAttachments))
	mux.HandleFunc("/attachments/download/", contextIterceptor(controllers.Download))
	mux.HandleFunc("/api/user/create", contextIterceptor(controllers.CreateUser))
	mux.HandleFunc("/api/user/edit", contextIterceptor(controllers.EditUser))
	mux.HandleFunc("/api/user/info", contextIterceptor(controllers.Info))
	mux.HandleFunc("/api/user/list", contextIterceptor(controllers.UserList))
	mux.HandleFunc("/api/domain/list", contextIterceptor(controllers.DomainList))
	mux.HandleFunc("/api/domain/add", contextIterceptor(controllers.AddDomain))
	mux.HandleFunc("/api/domain/del", contextIterceptor(controllers.DelDomain))
	mux.HandleFunc("/api/domain/dns", contextIterceptor(controllers.DomainDNSRecords))
	mux.HandleFunc("/api/domain/ssl/info", contextIterceptor(controllers.SSLInfo))
	mux.HandleFunc("/api/domain/ssl/issue", contextIterceptor(controllers.SSLIssue))
	mux.HandleFunc("/api/domain/ssl/records", contextIterceptor(controllers.SSLChallengeRecords))
	mux.HandleFunc("/api/domain/ssl/upload", contextIterceptor(controllers.SSLUpload))
	mux.HandleFunc("/api/system/restart", contextIterceptor(controllers.RestartServer))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if config.Instance != nil && config.Instance.HttpsEnabled != 2 {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func HttpStart() {
	mux := http.NewServeMux()

	HttpPort := 80
	if config.Instance.HttpPort > 0 {
		HttpPort = config.Instance.HttpPort
	}
	addr := netutil.Address(config.Instance.ListenMode, HttpPort)
	network := netutil.Network(config.Instance.ListenMode)

	if config.Instance.HttpsEnabled != 2 {
		// In redirect mode the ACME challenge must still be handled explicitly, otherwise the redirect breaks validation
		mux.HandleFunc("/.well-known/", controllers.AcmeChallenge)
		mux.HandleFunc("/api/ping", controllers.Ping)
		mux.HandleFunc("/", controllers.Interceptor)
		httpServer = &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  time.Second * 90,
			WriteTimeout: time.Second * 90,
		}
	} else {

		router(mux)

		log.Infof("HttpServer Start On %s", addr)
		httpServer = &http.Server{
			Addr:         addr,
			Handler:      session.Instance.LoadAndSave(mux),
			ReadTimeout:  time.Second * 90,
			WriteTimeout: time.Second * 90,
		}
	}

	listener, err := net.Listen(network, addr)
	if err != nil {
		panic(err)
	}
	err = httpServer.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
