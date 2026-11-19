package imap_server

import (
	"database/sql"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
)

func (s *serverSession) Login(username, pwd string) error {
	account := strings.TrimSpace(username)
	requestedDomain := ""
	if parts := strings.SplitN(account, "@", 2); len(parts) == 2 {
		account = parts[0]
		requestedDomain = strings.ToLower(strings.TrimSpace(parts[1]))
	} else if len(config.Instance.Domains) > 1 {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeAuthenticationFailed, Text: "Specify the mailbox domain with @"}
	}

	// A blocked source or account is refused before any account lookup or
	// password hashing, with the same reply a wrong password gets.
	if loginlimit.Blocked(s.remoteAddr, username) {
		log.WithContext(s.ctx).Debug("Login blocked by the failed sign-in limiter")
		return invalidCredentials()
	}

	var user models.User
	_, err := db.Instance.Where("account =? and disabled = 0", account).Get(&user)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.WithContext(s.ctx).Errorf("%+v", err)
	}
	if user.ID > 0 {
		valid, needsRehash := password.Verify(pwd, user.Password)
		if !valid || (requestedDomain != "" && user.Domain != "" && !strings.EqualFold(user.Domain, requestedDomain)) {
			user = models.User{}
		} else if needsRehash {
			_, _ = db.Instance.Table(&models.User{}).ID(user.ID).Cols("password").Update(map[string]interface{}{"password": password.Encode(pwd)})
		}
	}

	if user.ID > 0 {
		s.status = AUTHORIZED

		s.ctx.UserID = user.ID
		s.ctx.UserName = user.Name
		s.ctx.UserAccount = user.Account
		s.ctx.UserDomain = user.Domain
		loginlimit.Success(s.remoteAddr, username)
		log.WithContext(s.ctx).Debug("Login successful")

		return nil
	}

	loginlimit.Failure(s.remoteAddr, username)
	log.WithContext(s.ctx).Info("user not found")
	return invalidCredentials()
}

// invalidCredentials is the single authentication failure IMAP reports, so a
// blocked attempt looks exactly like a wrong password.
func invalidCredentials() error {
	return &imap.Error{
		Type: imap.StatusResponseTypeNo,
		Code: imap.ResponseCodeAuthenticationFailed,
		Text: "Invalid credentials (Failure)",
	}
}
