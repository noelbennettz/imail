package controllers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/session"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
)

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

func Login(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	source := loginSource(req)
	// A blocked address is refused before anything is read from the database,
	// with the same reply a wrong password gets.
	if loginlimit.BlockedSource(source) {
		log.WithContext(ctx).Debug("Web login blocked by the failed sign-in limiter")
		response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "aperror"), "").FPrint(w)
		return
	}

	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Errorf("%+v", err)
	}
	var reqData loginRequest
	if err = json.Unmarshal(reqBytes, &reqData); err != nil {
		log.Errorf("%+v", err)
	}

	var user models.User

	account := strings.TrimSpace(reqData.Account)
	requestedDomain := ""
	if parts := strings.SplitN(account, "@", 2); len(parts) == 2 {
		account = strings.TrimSpace(parts[0])
		requestedDomain = strings.ToLower(strings.TrimSpace(parts[1]))
	} else if len(config.Instance.Domains) > 1 {
		response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "login_requires_email"), "").FPrint(w)
		return
	}
	if account == "" || (requestedDomain == "" && strings.Contains(reqData.Account, "@")) {
		response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "aperror"), "").FPrint(w)
		return
	}

	_, err = db.Instance.Where("account =? and disabled=0", account).Get(&user)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Errorf("%+v", err)
	}
	if user.ID != 0 && requestedDomain != "" && user.Domain != "" && !strings.EqualFold(user.Domain, requestedDomain) {
		user = models.User{}
	}

	// Administrators get their own account counter, so a spray at the webmail
	// login cannot lock the console; the address counter is shared with every
	// plane and was already checked above.
	realm := loginlimit.RealmWeb
	if user.ID != 0 && user.IsAdmin == 1 {
		realm = loginlimit.RealmAdmin
	}
	if loginlimit.Blocked(realm, "", account) {
		log.WithContext(ctx).Debug("Web login blocked by the failed sign-in limiter")
		response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "aperror"), "").FPrint(w)
		return
	}

	if user.ID != 0 {
		valid, needsRehash := password.Verify(reqData.Password, user.Password)
		if !valid {
			user = models.User{}
		} else if needsRehash {
			if _, updateErr := db.Instance.Table(&models.User{}).ID(user.ID).Cols("password").Update(map[string]interface{}{"password": password.Encode(reqData.Password)}); updateErr != nil {
				log.WithContext(ctx).Errorf("upgrade password hash: %v", updateErr)
			}
		}
	}

	if user.ID != 0 {
		loginlimit.Success(realm, source, account)
		if err := session.Login(req.Context(), &user); err != nil {
			log.WithContext(ctx).Errorf("Create session: %v", err)
			response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
			return
		}

		domains := config.Instance.Domains
		domains = array.Difference(domains, []string{config.Instance.Domain})
		domains = append([]string{config.Instance.Domain}, domains...)

		response.NewSuccessResponse(map[string]any{
			"account":  user.Account,
			"name":     user.Name,
			"is_admin": user.IsAdmin,
			"domain":   user.Domain,
			"domains":  domains,
		}).FPrint(w)
	} else {
		loginlimit.Failure(realm, source, account)
		response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "aperror"), "").FPrint(w)
	}
}

// loginSource derives the limiter key of an HTTP login. The direct peer is
// used; when a reverse proxy on this machine forwards the request (the
// documented HTTPS-in-front setup), the client it names in X-Forwarded-For is
// used instead. A forwarded header from a remote peer is ignored, so an
// internet client cannot pick its own key.
func loginSource(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if client := net.ParseIP(strings.Trim(first, "[]")); client != nil {
				return client.String()
			}
		}
	}
	return host
}

func Logout(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if err := session.Instance.Destroy(ctx.Context); err != nil {
		response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
		return
	}
	response.NewSuccessResponse("Success").FPrint(w)
}
