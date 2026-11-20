package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/id"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

// bounceSender is the local part a delivery status notification is sent from.
const bounceSender = "MAILER-DAEMON"

// insertBounce stores a simple delivery status notification in the envelope
// sender's mailbox when a queue row finally fails and that sender is a mailbox
// this server hosts. It runs inside the transaction that records the failure, so
// a DSN is never written for a failure that was not committed.
//
// A bounce is only generated for a local sender: sending a failure report to a
// remote address would be backscatter, and replying to another bounce would
// loop.
func insertBounce(session *xorm.Session, entry *models.EmailQueue, reason string, now time.Time) error {
	account, domain := splitAddress(entry.EnvelopeFrom)
	if account == "" || domain == "" {
		return nil
	}
	if strings.EqualFold(account, bounceSender) {
		return nil
	}
	local := false
	for _, configured := range config.Instance.Domains {
		if strings.EqualFold(configured, domain) {
			local = true
			break
		}
	}
	if !local {
		return nil
	}

	var user models.User
	found, err := session.Table(&models.User{}).
		Where("LOWER(account)=? and disabled=0", strings.ToLower(account)).
		Get(&user)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	recipient := entry.EnvelopeFrom
	if recipients, err := Recipients(entry); err == nil && len(recipients) > 0 {
		recipient = recipients[0]
	}

	reportingHost := config.Instance.Domain
	mime := buildDSN(now, reportingHost, entry.EnvelopeFrom, recipient, reason)
	if mime == "" {
		return nil
	}

	from := bounceSender + "@" + domain
	to := parsemail.User{EmailAddress: entry.EnvelopeFrom}
	toJSON, err := json.Marshal([]parsemail.User{to})
	if err != nil {
		return err
	}
	msgID := fmt.Sprintf("%s.%s@%s", id.GenLogID(), id.GenLogID(), domain)
	bounce := models.Email{
		Type:         consts.EmailTypeReceive,
		Subject:      "Delivery Status Notification (Failure)",
		FromName:     "Mail Delivery System",
		FromAddress:  from,
		To:           string(toJSON),
		RawMIME:      mime,
		Size:         len(mime),
		SPFCheck:     1,
		DKIMCheck:    1,
		Status:       consts.EmailStatusWait,
		SendDate:     now,
		CronSendTime: now,
		CreateTime:   now,
		MsgID:        msgID,
	}
	if _, err := session.Insert(&bounce); err != nil {
		return err
	}
	if bounce.Id <= 0 {
		return fmt.Errorf("bounce insert returned no id")
	}
	relation := models.UserEmail{UserID: user.ID, EmailID: bounce.Id, Status: consts.EmailStatusWait, IsRead: 0}
	_, err = session.Insert(&relation)
	return err
}

// insertBounceBestEffort keeps a bounce problem from losing the delivery result:
// the queue row must end up failed even when the notification cannot be stored.
func insertBounceBestEffort(session *xorm.Session, entry *models.EmailQueue, reason string, now time.Time) {
	if err := insertBounce(session, entry, reason, now); err != nil {
		log.Errorf("outbox id=%d msg_id=%q could not store a delivery status notification: %v", entry.Id, entry.MsgID, err)
	}
}

func splitAddress(address string) (account, domain string) {
	address = strings.TrimSpace(address)
	account, domain, found := strings.Cut(address, "@")
	if !found {
		return "", ""
	}
	return strings.TrimSpace(account), strings.TrimSpace(domain)
}

// buildDSN renders a minimal RFC 3464 style report: a human readable part and a
// message/delivery-status part. It is deliberately small: one failed recipient,
// the reporting host and the remote reason.
func buildDSN(now time.Time, reportingHost, envelopeFrom, recipient, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "delivery failed"
	}
	const boundary = "imail-dsn-boundary"
	var body strings.Builder
	fmt.Fprintf(&body, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&body, "From: Mail Delivery System <%s@%s>\r\n", bounceSender, reportingHost)
	fmt.Fprintf(&body, "To: <%s>\r\n", envelopeFrom)
	body.WriteString("Subject: Delivery Status Notification (Failure)\r\n")
	body.WriteString("Auto-Submitted: auto-replied\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&body, "Content-Type: multipart/report; report-type=delivery-status; boundary=%q\r\n", boundary)
	body.WriteString("\r\n")
	fmt.Fprintf(&body, "--%s\r\n", boundary)
	body.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	body.WriteString("\r\n")
	fmt.Fprintf(&body, "Your message could not be delivered to the following recipient:\r\n\r\n")
	fmt.Fprintf(&body, "    <%s>\r\n\r\n", recipient)
	fmt.Fprintf(&body, "The remote server reported:\r\n\r\n    %s\r\n\r\n", reason)
	body.WriteString("No further attempts will be made.\r\n")
	fmt.Fprintf(&body, "--%s\r\n", boundary)
	body.WriteString("Content-Type: message/delivery-status\r\n")
	body.WriteString("\r\n")
	fmt.Fprintf(&body, "Reporting-MTA: dns; %s\r\n", reportingHost)
	body.WriteString("\r\n")
	fmt.Fprintf(&body, "Final-Recipient: rfc822; %s\r\n", recipient)
	body.WriteString("Action: failed\r\n")
	body.WriteString("Status: 5.0.0\r\n")
	fmt.Fprintf(&body, "Diagnostic-Code: smtp; %s\r\n", reason)
	fmt.Fprintf(&body, "--%s--\r\n", boundary)
	return body.String()
}
