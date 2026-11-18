package smtp_server

import (
	"bytes"
	"database/sql"
	oerrors "errors"
	"io"
	"net"
	"strings"
	"time"

	smtplib "github.com/emersion/go-smtp"
	"github.com/mileusna/spf"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/listen/imap_server"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/quota"
	"github.com/noelbennettz/imail/services/rule"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/send"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

// DropUnknownRecipientEmails is a code-level feature switch
// When set to true, mail addressed to non-existent users is dropped and never stored in the database
// This effectively prevents spam produced by scanners from landing in the first user's (admin's) mailbox
// Note: this switch can only be changed by editing the code, it is not exposed as user configuration
const DropUnknownRecipientEmails = true

func (s *Session) Data(r io.Reader) error {

	ctx := s.Ctx

	log.WithContext(ctx).Debugf("Email received")

	emailData, err := io.ReadAll(r)
	if err != nil {
		log.WithContext(ctx).Error("Unable to read email content", err)
		return err
	}
	s.RawEmailData = emailData

	log.WithContext(ctx).Debugf("%s", string(emailData))

	email := parsemail.NewEmailFromReader(s.To, bytes.NewReader(emailData), len(emailData))

	if s.From != "" {
		from := parsemail.BuilderUser(s.From)
		if email.From == nil {
			email.From = from
		}
		if email.From.EmailAddress != from.EmailAddress {
			// The from in the protocol does not match the from in the message content, treat it as spam
			//log.WithContext(s.Ctx).Infof("Spam, rejecting mail")
			//return nil
		}
	}

	// Receiving vs. sending: any authenticated session is treated as outbound.
	if s.Ctx.UserID > 0 {
		account, fromDomain := email.From.GetDomainAccount()
		if account != ctx.UserAccount && !ctx.IsAdmin {
			return oerrors.New("No Auth")
		}
		if ctx.UserDomain != "" && !strings.EqualFold(fromDomain, ctx.UserDomain) {
			return oerrors.New("No Auth: sender domain does not belong to this mailbox")
		}

		if email == nil {
			return nil
		}

		return s.deliverOutgoingEmail(email, send.Send)

	} else {
		// Inbound mail
		var dkimStatus, SPFStatus bool

		// DKIM validation
		dkimStatus = parsemail.Check(ctx, bytes.NewReader(emailData))

		SPFStatus = spfCheck(s.RemoteAddress.String(), email.Sender, email.Sender.EmailAddress)

		_, formDomain := email.From.GetDomainAccount()
		spoofed := array.InArray(formDomain, config.Instance.Domains) && SPFStatus == false
		if spoofed {
			dkimStatus = false
		}
		email.Authentication = parsemail.NewEmailAuthentication(SPFStatus, dkimStatus)

		email.Authentication = parsemail.NewEmailAuthentication(SPFStatus, dkimStatus)
		// Spoofed local-domain mail is classified as junk, not deleted.
		if spoofed {
			email.Status = int(consts.EmailStatusJunk)
		}

		users, dbEmail, saveErr := saveEmail(ctx, len(emailData), emailData, email, 0, 0, s.To, SPFStatus, dkimStatus)
		if saveErr != nil {
			log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=inbound msg_id=%q error=%v", email.MsgID, saveErr)
			var smtpErr *smtplib.SMTPError
			if oerrors.As(saveErr, &smtpErr) {
				return smtpErr
			}
			return localPersistenceSMTPError()
		}

		if email.MessageId > 0 {
			log.WithContext(ctx).Debugf("Start executing mail rules!")
			for _, user := range users {
				// Execute the mail rules
				rs := rule.GetAllRules(ctx, user.ID)
				for _, r := range rs {
					if rule.MatchRule(ctx, r, email) {
						rule.DoRule(ctx, r, email, user, emailData)
					}
				}
			}
		}

		log.WithContext(ctx).Debugf("Persisting received mail completed")
		var ue []*models.UserEmail
		err = db.Instance.Table(&models.UserEmail{}).Where("email_id=?", email.MessageId).Find(&ue)
		if err != nil {
			log.WithContext(ctx).Errorf("sql Error :%+v", err)
		}

		// Notify via IDLE command
		for _, user := range users {
			imap_server.IdleNotice(ctx, user.ID, dbEmail)
		}

	}

	return nil
}

func saveEmail(ctx *context.Context, size int, rawMIME []byte, email *parsemail.Email, sendUserID int, emailType int, reallyTo []string, SPFStatus, dkimStatus bool) ([]*models.User, *models.Email, error) {
	if email == nil {
		return nil, nil, oerrors.New("email is missing")
	}
	if email.From == nil {
		return nil, nil, oerrors.New("email sender is missing")
	}
	if emailType == int(consts.EmailTypeReceive) && email.Authentication != nil {
		SPFStatus = email.Authentication.SPFPassed
		dkimStatus = email.Authentication.DKIMPassed
	}

	users, drop, err := resolveIncomingUsers(ctx, email, emailType, reallyTo, SPFStatus, dkimStatus)
	if err != nil || drop {
		return users, nil, err
	}

	msgID := email.MsgID
	if msgID == "" {
		msgID = parsemail.GenerateMsgID(config.Instance.Domain)
	}
	if email.Size == 0 {
		email.Size = size
	}

	modelEmail := models.Email{
		Type:         cast.ToInt8(emailType),
		Subject:      email.Subject,
		ReplyTo:      json2string(email.ReplyTo),
		FromName:     email.From.Name,
		FromAddress:  email.From.EmailAddress,
		To:           json2string(email.To),
		Bcc:          json2string(email.Bcc),
		Cc:           json2string(email.Cc),
		Text:         sql.NullString{String: string(email.Text), Valid: true},
		Html:         sql.NullString{String: string(email.HTML), Valid: true},
		Sender:       json2string(email.Sender),
		Attachments:  json2string(email.Attachments),
		RawMIME:      string(rawMIME),
		Size:         email.Size,
		SPFCheck:     boolInt8(SPFStatus),
		DKIMCheck:    boolInt8(dkimStatus),
		SendUserID:   sendUserID,
		SendDate:     time.Now(),
		Status:       cast.ToInt8(email.Status),
		CreateTime:   time.Now(),
		CronSendTime: time.Now(),
		MsgID:        msgID,
	}

	var userIDs []int
	if emailType == int(consts.EmailTypeReceive) {
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
	} else {
		modelEmail.Status = consts.EmailStatusDeliveryPending
		modelEmail.Error = sql.NullString{String: deliveryPendingError, Valid: true}
		userIDs = []int{sendUserID}
	}

	// Enforce mailbox capacity before writing the message.
	quotaUsers := users
	if emailType != int(consts.EmailTypeReceive) {
		var owner models.User
		if _, ownerErr := db.Instance.ID(sendUserID).Get(&owner); ownerErr == nil {
			quotaUsers = []*models.User{&owner}
		}
	}
	if err := quota.Check(quotaUsers, modelEmail.Size); err != nil {
		log.WithContext(ctx).Warnf("quota rejected message for %v: %v", userIDs, err)
		return users, nil, &smtplib.SMTPError{
			Code:         452,
			EnhancedCode: smtplib.EnhancedCode{4, 2, 2},
			Message:      "Mailbox quota exceeded",
		}
	}

	log.WithContext(ctx).Debugf("Saving to database!")
	saved, err := persistInitialAudit(ctx, modelEmail, userIDs)
	if err != nil {
		return users, nil, err
	}
	email.MessageId = cast.ToInt64(saved.Id)
	email.MsgID = saved.MsgID
	return users, saved, nil
}

func spfCheck(remoteAddress string, sender *parsemail.User, senderString string) bool {
	// spf validation. The remote address is host:port on both stacks
	// ("1.2.3.4:25" and "[2001:db8::1]:25"), and an IPv6 zone may be attached.
	host := remoteAddress
	if h, _, err := net.SplitHostPort(remoteAddress); err == nil {
		host = h
	}
	if zone := strings.LastIndex(host, "%"); zone >= 0 {
		host = host[:zone]
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		log.Debugf("spfCheck: cannot parse remote address %q", remoteAddress)
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}

	tmp := strings.Split(sender.EmailAddress, "@")
	if len(tmp) < 2 {
		return false
	}

	res := spf.CheckHost(ip, tmp[1], senderString, "")

	if res == spf.None || res == spf.Pass {
		// spf validation passed
		return true
	}
	return false
}
