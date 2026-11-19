package smtp_server

import (
	"database/sql"
	"errors"
	"net"
	"strings"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/id"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
)

// The Backend implements SMTP server methods.
type Backend struct{}

func (bkd *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	remoteAddress := conn.Conn().RemoteAddr()
	ctx := &context.Context{}
	ctx.SetValue(context.LogID, id.GenLogID())
	log.WithContext(ctx).Debugf("New SMTP connection")

	return &Session{
		RemoteAddress: remoteAddress,
		Ctx:           ctx,
	}, nil
}

// A Session is returned after EHLO.
type Session struct {
	RemoteAddress net.Addr
	User          string
	From          string
	To            []string
	Ctx           *context.Context
	RawEmailData  []byte
}

// AuthMechanisms returns a slice of available auth mechanisms
// supported in this example.
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

// Auth is the handler for supported authenticators.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	log.WithContext(s.Ctx).Debugf("Auth :%s", mech)
	if mech == sasl.Plain {
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return s.AuthPlain(username, password)
		}), nil
	}

	if mech == sasl.Login {
		return NewLoginServer(func(username, password string) error {
			return s.AuthPlain(username, password)
		}), nil
	}

	return nil, errors.New("Auth Not Supported")
}

func (s *Session) AuthPlain(username, pwd string) error {
	log.WithContext(s.Ctx).Debugf("SMTP authentication attempt for %q", username)

	s.User = username

	var user models.User

	account := username
	requestedDomain := ""
	infos := strings.Split(username, "@")
	if len(infos) > 1 {
		account = infos[0]
		requestedDomain = strings.ToLower(infos[1])
	}

	if requestedDomain == "" && len(config.Instance.Domains) > 1 {
		return errors.New("specify the mailbox domain with @")
	}

	// A blocked source or account gets the same reply as a wrong password,
	// before any account lookup or password hashing happens.
	source := loginlimit.SourceKey(s.RemoteAddress)
	if loginlimit.Blocked(source, username) {
		log.WithContext(s.Ctx).Debugf("SMTP authentication blocked by the failed sign-in limiter")
		return errors.New("password error")
	}

	_, err := db.Instance.Where("account =? and disabled=0", account).Get(&user)
	if err != nil && err != sql.ErrNoRows {
		log.Errorf("%+v", err)
	}
	if user.ID > 0 {
		valid, needsRehash := password.Verify(pwd, user.Password)
		if !valid {
			user = models.User{}
		} else if requestedDomain != "" && user.Domain != "" && !strings.EqualFold(user.Domain, requestedDomain) {
			user = models.User{}
		} else if needsRehash {
			_, _ = db.Instance.Table(&models.User{}).ID(user.ID).Cols("password").Update(map[string]interface{}{"password": password.Encode(pwd)})
		}
	}

	if user.ID > 0 {
		s.Ctx.UserAccount = user.Account
		s.Ctx.UserID = user.ID
		s.Ctx.UserName = user.Name
		s.Ctx.UserDomain = user.Domain
		s.Ctx.IsAdmin = user.IsAdmin == 1

		loginlimit.Success(source, username)
		log.WithContext(s.Ctx).Debugf("SMTP authentication succeeded for user %d", user.ID)
		return nil
	}

	loginlimit.Failure(source, username)
	log.WithContext(s.Ctx).Debugf("SMTP authentication failed for %q", username)
	return errors.New("password error")
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.WithContext(s.Ctx).Debugf("Mail Success %+v %+v", from, opts)
	s.From = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.WithContext(s.Ctx).Debugf("Rcpt Success %+v", to)

	s.To = append(s.To, to)
	return nil
}

func (s *Session) Reset() {
	s.From = ""
	s.To = nil
}

func (s *Session) Logout() error {
	return nil
}
