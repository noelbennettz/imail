package pop3_server

import (
	"database/sql"
	errors2 "errors"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/del_email"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/services/list"
	"github.com/noelbennettz/imail/services/loginlimit"
	"github.com/noelbennettz/imail/third_party/gopop"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/id"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

type action struct{}

// pop3Maildrop is the INBOX snapshot exposed over POP3. POP3 has no folders, so
// only received, non-deleted messages in INBOX are part of the maildrop.
func pop3Maildrop(session *gopop.Session) []listItem {
	emailList, _ := list.GetEmailList(session.Ctx.(*context.Context), dto.SearchTag{Type: consts.EmailTypeReceive, Status: -1, GroupId: -1}, "", true, 0, 99999)
	items := make([]listItem, 0, len(emailList))
	for _, info := range emailList {
		items = append(items, listItem{Id: cast.ToInt64(info.Id), Size: cast.ToInt64(info.Size)})
	}
	return items
}

// ensureMaildrop assigns the stable 1..N message numbers that POP3 clients
// expect. The mapping is built once per session, so a message number cannot
// change while the session is open.
func ensureMaildrop(session *gopop.Session) map[int64]int64 {
	if session.Messages != nil {
		return session.Messages
	}
	session.Messages = map[int64]int64{}
	session.Sizes = map[int64]int64{}
	for i, item := range pop3Maildrop(session) {
		number := int64(i + 1)
		session.Messages[number] = item.Id
		session.Sizes[number] = item.Size
	}
	return session.Messages
}

func emailIDFor(session *gopop.Session, number int64) (int64, bool) {
	id, ok := ensureMaildrop(session)[number]
	return id, ok
}

// Custom non-standard command
func (a action) Custom(session *gopop.Session, cmd string, args []string) ([]string, error) {
	if session.Ctx == nil {
		tc := &context.Context{}
		tc.SetValue(context.LogID, id.GenLogID())
		session.Ctx = tc
	}

	log.WithContext(session.Ctx).Debugf("not supported cmd request! cmd:%s args:%v", cmd, args)
	return nil, errors2.New("not supported cmd request")
}

// Capa lists the commands the server supports
func (a action) Capa(session *gopop.Session) ([]string, error) {
	if session.Ctx == nil {
		tc := &context.Context{}
		tc.SetValue(context.LogID, id.GenLogID())
		session.Ctx = tc
	}

	if session.InTls {
		log.WithContext(session.Ctx).Debugf("POP3 CMD: CAPA With Tls")
	} else {
		log.WithContext(session.Ctx).Debugf("POP3 CMD: CAPA Without Tls")
	}

	ret := []string{
		"USER",
		"PASS",
		"TOP",
		"STAT",
		"UIDL",
		"LIST",
		"RETR",
		"DELE",
		"REST",
		"NOOP",
		"QUIT",
	}
	if !session.InTls {
		ret = append(ret, "STLS")
	}

	log.WithContext(session.Ctx).Debugf("CAPA \n %+v", ret)

	return ret, nil
}

// User submits the login user name
func (a action) User(session *gopop.Session, username string) error {
	if session.Ctx == nil {
		tc := &context.Context{}
		tc.SetValue(context.LogID, id.GenLogID())
		session.Ctx = tc
	}
	log.WithContext(session.Ctx).Debugf("POP3 CMD: USER, Args:%s", username)

	username = strings.TrimSpace(username)
	log.WithContext(session.Ctx).Debugf("POP3 User %s", username)

	session.User = username
	return nil
}

// Pass submits the password for verification
func (a action) Pass(session *gopop.Session, pwd string) error {
	if session.Ctx == nil {
		tc := &context.Context{}
		tc.SetValue(context.LogID, id.GenLogID())
		session.Ctx = tc
	}

	if !session.InTls {
		return errors2.New("TLS is required for POP3 authentication")
	}
	log.WithContext(session.Ctx).Debugf("POP3 PASS attempt, User:%s", session.User)

	source := loginlimit.SourceKeyConn(session.Conn)
	// A blocked source or account is refused before any account lookup or
	// password hashing, with the same reply a wrong password gets.
	if loginlimit.Blocked(loginlimit.RealmMail, source, session.User) {
		log.WithContext(session.Ctx).Debugf("POP3 authentication blocked by the failed sign-in limiter")
		return errors2.New("password error")
	}

	account := session.User
	requestedDomain := ""
	if parts := strings.SplitN(account, "@", 2); len(parts) == 2 {
		account = parts[0]
		requestedDomain = strings.ToLower(strings.TrimSpace(parts[1]))
	} else if len(config.Instance.Domains) > 1 {
		return errors2.New("specify the mailbox domain with @")
	}

	var user models.User
	_, err := db.Instance.Where("account =? and disabled = 0", account).Get(&user)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.WithContext(session.Ctx.(*context.Context)).Errorf("%+v", err)
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
		session.Status = gopop.TRANSACTION

		session.Ctx.(*context.Context).UserID = user.ID
		session.Ctx.(*context.Context).UserName = user.Name
		session.Ctx.(*context.Context).UserAccount = user.Account
		session.Ctx.(*context.Context).UserDomain = user.Domain

		loginlimit.Success(loginlimit.RealmMail, source, session.User)
		return nil
	}

	loginlimit.Failure(loginlimit.RealmMail, source, session.User)
	return errors2.New("password error")
}

// Apop APOP login command. APOP requires the plaintext password digest and
// cannot be used with PBKDF2 password storage.
func (a action) Apop(session *gopop.Session, username, digest string) error {
	return errors2.New("APOP is not supported")
}

// Stat queries the number of messages
func (a action) Stat(session *gopop.Session) (msgNum, msgSize int64, err error) {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: STAT")

	maildrop := ensureMaildrop(session)
	for number := int64(1); number <= int64(len(maildrop)); number++ {
		msgNum++
		msgSize += session.Sizes[number]
	}
	log.WithContext(session.Ctx).Debugf("POP3 STAT RETURT : %d,%d", msgNum, msgSize)
	return msgNum, msgSize, nil
}

// Uidl queries the unique identifier of a message
func (a action) Uidl(session *gopop.Session, msg string) ([]gopop.UidlItem, error) {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: UIDL ,Args:%s", msg)

	if msg != "" {
		number := cast.ToInt64(msg)
		emailID, ok := emailIDFor(session, number)
		if !ok {
			return nil, errors.New("no such message")
		}
		return []gopop.UidlItem{{Id: number, UnionId: cast.ToString(emailID)}}, nil
	}

	maildrop := ensureMaildrop(session)
	ret := make([]gopop.UidlItem, 0, len(maildrop))
	for number := int64(1); number <= int64(len(maildrop)); number++ {
		ret = append(ret, gopop.UidlItem{Id: number, UnionId: cast.ToString(maildrop[number])})
	}

	log.WithContext(session.Ctx).Debugf("Uidl \n %+v", ret)
	return ret, nil
}

type listItem struct {
	Id   int64 `json:"id"`
	Size int64 `json:"size"`
}

// List lists the messages
func (a action) List(session *gopop.Session, msg string) ([]gopop.MailInfo, error) {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: LIST ,Args:%s", msg)

	_ = ensureMaildrop(session)
	sizes := session.Sizes
	if sizes == nil {
		sizes = map[int64]int64{}
	}

	var numbers []int64
	if msg != "" {
		number := cast.ToInt64(msg)
		if number <= 0 {
			return nil, errors.New("params error")
		}
		if _, ok := sizes[number]; !ok {
			return nil, errors.New("no such message")
		}
		numbers = []int64{number}
	} else {
		for number := int64(1); number <= int64(len(sizes)); number++ {
			numbers = append(numbers, number)
		}
	}

	ret := make([]gopop.MailInfo, 0, len(numbers))
	for _, number := range numbers {
		ret = append(ret, gopop.MailInfo{Id: number, Size: sizes[number]})
	}

	log.WithContext(session.Ctx).Debugf("List \n %+v", ret)
	return ret, nil
}

// Retr fetches the message details
func (a action) Retr(session *gopop.Session, id int64) (string, int64, error) {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: RETR ,Args:%d", id)
	emailID, ok := emailIDFor(session, id)
	if !ok {
		return "", 0, errors.New("no such message")
	}
	email, err := detail.GetEmailDetail(session.Ctx.(*context.Context), cast.ToInt(emailID), false)
	if err != nil {
		log.WithContext(session.Ctx.(*context.Context)).Errorf("%+v", err)
		return "", 0, errors.New("server error")
	}

	ret := parsemail.NewEmailFromModel(email.Email).BuildBytes(session.Ctx.(*context.Context), false)
	log.WithContext(session.Ctx).Debugf("Retr \n %+v", string(ret))
	return string(ret), cast.ToInt64(len(ret)), nil
}

// Delete deletes a message
func (a action) Delete(session *gopop.Session, id int64) error {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: DELE ,Args:%d", id)

	emailID, ok := emailIDFor(session, id)
	if !ok {
		return errors.New("no such message")
	}
	session.DeleteIds = append(session.DeleteIds, emailID)
	session.DeleteIds = array.Unique(session.DeleteIds)
	return nil
}

func (a action) Rest(session *gopop.Session) error {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: REST ")
	session.DeleteIds = []int64{}
	return nil
}

func (a action) Top(session *gopop.Session, id int64, n int) (string, error) {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: TOP %d %d", id, n)
	emailID, ok := emailIDFor(session, id)
	if !ok {
		return "", errors.New("no such message")
	}
	email, err := detail.GetEmailDetail(session.Ctx.(*context.Context), cast.ToInt(emailID), false)
	if err != nil {
		log.WithContext(session.Ctx.(*context.Context)).Errorf("%+v", err)
		return "", errors2.New("server error")
	}

	ret := parsemail.NewEmailFromModel(email.Email).BuildBytes(session.Ctx.(*context.Context), false)
	res := strings.Split(string(ret), "\n")
	headerEndLine := len(res) - 1
	for i, re := range res {
		if re == "\r" {
			headerEndLine = i
			break
		}
	}
	if len(res) <= headerEndLine+n+1 {
		return string(ret), nil
	}

	lines := array.Join(res[0:headerEndLine+n+1], "\n")
	log.WithContext(session.Ctx).Debugf("Top \n %+v", lines)
	return lines, nil
}

func (a action) Noop(session *gopop.Session) error {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: NOOP ")
	return nil
}

func (a action) Quit(session *gopop.Session) error {
	log.WithContext(session.Ctx).Debugf("POP3 CMD: QUIT ")

	var DelIds []int

	if len(session.DeleteIds) > 0 {
		for _, delId := range session.DeleteIds {
			DelIds = append(DelIds, cast.ToInt(delId))
		}

		del_email.DelEmail(session.Ctx.(*context.Context), DelIds, false)
	}

	return nil
}
