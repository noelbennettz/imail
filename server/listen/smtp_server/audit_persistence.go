package smtp_server

import (
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"

	smtp "github.com/emersion/go-smtp"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
	"xorm.io/builder"
	"xorm.io/xorm"
)

const deliveryPendingError = "delivery result pending; outcome unknown after interruption"

type outgoingSender func(*context.Context, *parsemail.Email) (error, map[string]error)

func (s *Session) deliverOutgoingEmail(email *parsemail.Email, sender outgoingSender) error {
	ctx := s.Ctx
	_, saved, err := saveEmail(ctx, email.Size, s.RawEmailData, email, ctx.UserID, int(consts.EmailTypeSend), nil, true, true)
	if err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=outbound msg_id=%q error=%v", email.MsgID, err)
		var smtpErr *smtp.SMTPError
		if stderrors.As(err, &smtpErr) {
			return smtpErr
		}
		return localPersistenceSMTPError()
	}

	deliveryErr, domainErrors := sender(ctx, email)

	targetStatus := consts.EmailStatusSent
	var errorText any
	if deliveryErr != nil {
		targetStatus = consts.EmailStatusFail
		errorText = deliveryErr.Error()
	}

	if err = persistDeliveryResult(ctx, saved.Id, targetStatus, errorText); err != nil {
		log.WithContext(ctx).Errorf(
			"CRITICAL audit_event=delivery_result_persist_failed email_id=%d msg_id=%q target_status=%d downstream_accepted=%t error=%v",
			saved.Id, saved.MsgID, targetStatus, deliveryErr == nil, err,
		)
	}

	// If the remote MX accepted DATA, returning a local 451 here would cause a
	// duplicate delivery. The persistence failure is retained as pending state
	// and a CRITICAL log; the SMTP result must still reflect the remote result.
	return downstreamDeliverySMTPError(deliveryErr, domainErrors)
}

func persistDeliveryResult(ctx *context.Context, emailID int, status int8, errorText any) error {
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		result, err := session.Exec(db.WithContext(ctx, `update email
			set status=?, error=?, update_time=CURRENT_TIMESTAMP
			where id=?`), status, errorText, emailID)
		if err != nil {
			return err
		}
		if err = requireOneAffected(result, "update email delivery result"); err != nil {
			return err
		}

		result, err = session.Exec(db.WithContext(ctx, `update user_email
			set status=? where email_id=?`), status, emailID)
		if err != nil {
			return err
		}
		return requireOneAffected(result, "update user_email delivery result")
	})
}

func requireOneAffected(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: read affected rows: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s: affected %d rows, want 1", operation, affected)
	}
	return nil
}

func localPersistenceSMTPError() error {
	return &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 3, 0},
		Message:      "Temporary local persistence failure",
	}
}

func persistInitialAudit(ctx *context.Context, modelEmail models.Email, userIDs []int) (*models.Email, error) {
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
