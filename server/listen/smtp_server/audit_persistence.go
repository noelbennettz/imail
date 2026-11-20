package smtp_server

import (
	"encoding/json"
	stderrors "errors"
	"strings"
	"time"

	smtp "github.com/emersion/go-smtp"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/send"
	log "github.com/sirupsen/logrus"
	"xorm.io/builder"
	"xorm.io/xorm"
)

const deliveryPendingError = "delivery result pending; outcome unknown after interruption"

// outboundQueue describes the queue rows that are written with an outbound
// email: the envelope sender and every envelope recipient. Local mailboxes are
// queued too, so the message is fully durable once the transaction commits.
type outboundQueue struct {
	EnvelopeFrom string
	Recipients   []string
}

// outboundRecipients returns every envelope recipient of the message, deduped
// and trimmed, including mailboxes this server hosts.
func outboundRecipients(email *parsemail.Email) []string {
	var recipients []*parsemail.User
	for _, list := range [][]*parsemail.User{email.To, email.Cc, email.Bcc} {
		recipients = append(recipients, list...)
	}
	_, all := send.SplitRecipients(recipients, nil)
	return all
}

func (s *Session) deliverOutgoingEmail(email *parsemail.Email) error {
	ctx := s.Ctx
	recipients := outboundRecipients(email)
	var enqueue *outboundQueue
	if len(recipients) > 0 {
		enqueue = &outboundQueue{
			EnvelopeFrom: email.From.EmailAddress,
			Recipients:   recipients,
		}
	}

	_, saved, err := saveEmail(ctx, email.Size, s.RawEmailData, email, ctx.UserID, int(consts.EmailTypeSend), nil, true, true, enqueue)
	if err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=outbound msg_id=%q error=%v", email.MsgID, err)
		var smtpErr *smtp.SMTPError
		if stderrors.As(err, &smtpErr) {
			return smtpErr
		}
		return localPersistenceSMTPError()
	}

	if enqueue != nil {
		log.WithContext(ctx).Infof("Outbox queued message %d msg_id=%q recipients=%d", saved.Id, saved.MsgID, len(enqueue.Recipients))
	}

	// The message row and every queue row committed in one transaction, so the
	// client can be told the message was accepted: no delivery happens inline
	// any more, and nothing can be lost between the commit and a delivery.
	return nil
}

func localPersistenceSMTPError() error {
	return &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 3, 0},
		Message:      "Temporary local persistence failure",
	}
}

func persistInitialAudit(ctx *context.Context, modelEmail models.Email, userIDs []int, enqueue *outboundQueue) (*models.Email, error) {
	var saved models.Email
	err := db.Transaction(db.Instance, func(session *xorm.Session) error {
		candidate := modelEmail
		candidate.Id = 0
		if _, err := session.Insert(&candidate); err != nil {
			return err
		}
		if candidate.Id <= 0 {
			return stderrors.New("email insert returned no id")
		}
		for _, userID := range userIDs {
			relation := models.UserEmail{EmailID: candidate.Id, UserID: userID, Status: candidate.Status}
			if _, err := session.Insert(&relation); err != nil {
				return err
			}
		}
		// The queue row commits with the message, so an accepted message is
		// never lost between the audit insert and the enqueue.
		if enqueue != nil && len(enqueue.Recipients) > 0 {
			// One row per remote domain: a domain that accepts the message is
			// never contacted again while another domain retries.
			entries, err := queue.Entries(candidate.Id, candidate.MsgID, enqueue.EnvelopeFrom, enqueue.Recipients, time.Now())
			if err != nil {
				return err
			}
			for i := range entries {
				if _, err := session.Insert(&entries[i]); err != nil {
					return err
				}
			}
		}
		saved = candidate
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func resolveIncomingUsers(ctx *context.Context, email *parsemail.Email, emailType int, reallyTo []string, SPFStatus, dkimStatus bool) ([]*models.User, bool, error) {
	if emailType != int(consts.EmailTypeReceive) {
		return nil, false, nil
	}

	accounts := incomingAccounts(email, reallyTo)
	domainsByAccount := incomingDomains(email, reallyTo)
	var users []*models.User
	if len(accounts) > 0 {
		where, params, err := builder.ToSQL(builder.In("LOWER(account)", accounts))
		if err != nil {
			return nil, false, err
		}
		if err = db.Instance.Table(&models.User{}).Where(where, params...).Find(&users); err != nil {
			return nil, false, err
		}
	}
	// When a mailbox is bound to a domain, only accept mail addressed to
	// that domain. Legacy accounts without a domain stay domain-agnostic.
	filtered := users[:0]
	for _, user := range users {
		if user.Domain == "" || domainsByAccount[strings.ToLower(user.Account)][strings.ToLower(user.Domain)] {
			filtered = append(filtered, user)
		}
	}
	users = filtered
	if len(users) > 0 {
		return users, false, nil
	}

	if DropUnknownRecipientEmails &&
		((config.Instance.SpamFilterLevel == 1 && !SPFStatus && !dkimStatus) ||
			(config.Instance.SpamFilterLevel == 2 && !SPFStatus) ||
			(config.Instance.SpamFilterLevel == 3 && !dkimStatus)) {
		log.WithContext(ctx).Infoln("Spam, rejecting mail")
		log.WithContext(ctx).Infof("Recipient does not exist and DKIM validation failed, dropping mail: %s -> %v", email.From.EmailAddress, accounts)
		return nil, true, nil
	}

	log.WithContext(ctx).Infof("Recipient does not exist but DKIM validation passed, handing over to the admin: %s -> %v", email.From.EmailAddress, accounts)
	if err := db.Instance.Table(&models.User{}).Where("is_admin=1").Find(&users); err != nil {
		return nil, false, err
	}
	if len(users) == 0 {
		return nil, false, stderrors.New("no administrator available for unknown recipient")
	}
	return users, false, nil
}

func incomingAccounts(email *parsemail.Email, reallyTo []string) []string {
	var accounts []string
	seen := map[string]bool{}
	appendAccount := func(user *parsemail.User) {
		if user == nil {
			return
		}
		name, _ := user.GetDomainAccount()
		name = strings.ToLower(name)
		if name != "" && !seen[name] {
			seen[name] = true
			accounts = append(accounts, name)
		}
	}
	if len(reallyTo) > 0 {
		for _, recipient := range reallyTo {
			account := parsemail.BuilderUser(recipient)
			if account == nil {
				continue
			}
			name, domain := account.GetDomainAccount()
			if array.InArray(domain, config.Instance.Domains) && name != "" {
				appendAccount(account)
			}
		}
		return accounts
	}
	for _, recipients := range [][]*parsemail.User{email.To, email.Cc, email.Bcc} {
		for _, user := range recipients {
			appendAccount(user)
		}
	}
	return accounts
}

// incomingDomains maps a lower-cased account to the set of domains it was
// addressed at in this transaction.
func incomingDomains(email *parsemail.Email, reallyTo []string) map[string]map[string]bool {
	domains := map[string]map[string]bool{}
	record := func(user *parsemail.User) {
		if user == nil {
			return
		}
		name, domain := user.GetDomainAccount()
		if name == "" || domain == "" {
			return
		}
		name = strings.ToLower(name)
		domain = strings.ToLower(domain)
		if domains[name] == nil {
			domains[name] = map[string]bool{}
		}
		domains[name][domain] = true
	}
	if len(reallyTo) > 0 {
		for _, recipient := range reallyTo {
			record(parsemail.BuilderUser(recipient))
		}
		return domains
	}
	for _, recipients := range [][]*parsemail.User{email.To, email.Cc, email.Bcc} {
		for _, user := range recipients {
			record(user)
		}
	}
	return domains
}

func boolInt8(value bool) int8 {
	if value {
		return 1
	}
	return 0
}

func json2string(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
