package send

import (
	"errors"
	"fmt"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/async"
	"github.com/noelbennettz/imail/utils/consts"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/netutil"
	"github.com/noelbennettz/imail/utils/smtp"
	log "github.com/sirupsen/logrus"
	"net"
	"net/textproto"
	"strings"
	"sync"
)

type mxDomain struct {
	domain string
	mxHost string
}

// mxFailoverEntry records all MX hosts (ordered by priority) of one recipient domain.
type mxFailoverEntry struct {
	domain  string
	mxHosts []string
}

type temporaryMXFallbackError struct {
	lookupErr   error
	fallbackErr error
}

func (e *temporaryMXFallbackError) Error() string {
	return fmt.Sprintf("temporary MX lookup failed (%v); fallback delivery failed: %v", e.lookupErr, e.fallbackErr)
}

func (e *temporaryMXFallbackError) Unwrap() error {
	return e.lookupErr
}

// Forward forwards an email
func Forward(ctx *context.Context, e *parsemail.Email, forwardAddress string, user *models.User) error {

	log.WithContext(ctx).Debugf("Start forwarding email")

	b := e.ForwardBuildBytes(ctx, user, forwardAddress)

	log.WithContext(ctx).Debugf("%s", b)

	from := user.Account + "@" + config.Instance.Domains[0]
	return forwardData(ctx, config.Instance.Domains[0], b, forwardAddress, from)
}

func ForwardRaw(ctx *context.Context, e *parsemail.Email, rawEmailData []byte, forwardAddress string, user *models.User) error {
	log.WithContext(ctx).Debugf("Start raw email forwarding")

	from := user.Account + "@" + config.Instance.Domains[0]
	return forwardData(ctx, config.Instance.Domains[0], rawEmailData, forwardAddress, from)
}

func forwardData(ctx *context.Context, fromDomain string, data []byte, forwardAddress string, from string) error {
	var to []*parsemail.User
	to = []*parsemail.User{
		{EmailAddress: forwardAddress},
	}

	err, _ := doSend(ctx, fromDomain, data, to, from)
	return err
}

func Send(ctx *context.Context, e *parsemail.Email) (error, map[string]error) {

	_, fromDomain := e.From.GetDomainAccount()

	b := e.BuildBytes(ctx, true)

	var to []*parsemail.User
	to = append(append(append(to, e.To...), e.Cc...), e.Bcc...)

	return doSend(ctx, fromDomain, b, to, e.From.EmailAddress)

}

func doSend(ctx *context.Context, fromDomain string, data []byte, to []*parsemail.User, from string) (error, map[string]error) {

	// Group by domain
	toByDomain := map[mxDomain][]*parsemail.User{}
	mxLookupErrors := map[mxDomain]error{}
	// mxFailoverMap holds the full MX list (ordered by priority) of each domain, used for failover
	mxFailoverMap := map[string][]string{}
	for _, s := range to {
		args := strings.Split(s.EmailAddress, "@")
		if len(args) == 2 {
			if args[1] == consts.TEST_DOMAIN {
				// For testing
				address := mxDomain{
					domain: "localhost",
					mxHost: netutil.Loopback(config.Instance.ListenMode),
				}
				toByDomain[address] = append(toByDomain[address], s)
			} else {
				// Query the dns mx records
				mxInfo, lookupErr := net.LookupMX(args[1])
				address := mxDomain{
					domain: "smtp." + args[1],
					mxHost: "smtp." + args[1],
				}
				if lookupErr != nil {
					log.WithContext(ctx).Errorf("%s domain mx record lookup failed, check whether the mailbox exists!", s.EmailAddress)
				}
				if len(mxInfo) > 0 {
					address = mxDomain{
						domain: args[1],
						mxHost: strings.TrimSuffix(strings.TrimSpace(mxInfo[0].Host), "."),
					}
					// Keep all MX hosts (net.LookupMX already sorts by priority).
					// A trailing dot is stripped here so the same name is used for
					// dialing and for TLS verification.
					allHosts := make([]string, 0, len(mxInfo))
					for _, mx := range mxInfo {
						allHosts = append(allHosts, strings.TrimSuffix(strings.TrimSpace(mx.Host), "."))
					}
					mxFailoverMap[args[1]] = allHosts
				}
				if lookupErr != nil {
					mxLookupErrors[address] = lookupErr
				}
				toByDomain[address] = append(toByDomain[address], s)
			}
		} else {
			log.WithContext(ctx).Errorf("email address parse error! %s", s)
			continue
		}
	}

	var errEmailAddress []string
	var errEmailAddressMu sync.Mutex

	errMap := sync.Map{}

	as := async.New(ctx)
	for domain, tos := range toByDomain {
		domain := domain
		tos := tos
		mxLookupErr := mxLookupErrors[domain]
		as.WaitProcess(func(p any) {
			recordFailure := func(err error) {
				err = deliveryFailureCause(mxLookupErr, err)
				log.WithContext(ctx).Errorf("%v email delivery failed%+v", tos, err)

				errEmailAddressMu.Lock()
				for _, user := range tos {
					errEmailAddress = append(errEmailAddress, user.EmailAddress)
				}
				errEmailAddressMu.Unlock()

				errMap.Store(domain.domain, err)
			}

			if domain.domain == "localhost" {
				err := smtp.SendMailUnsafe("", smtp.Address(domain.mxHost, 25), nil, from, fromDomain, buildAddress(tos), data)
				if err != nil {
					recordFailure(err)
				}
				return
			}

			// Fetch all MX hosts of the domain (ordered by priority) for failover
			mxHosts := mxFailoverMap[domain.domain]
			if len(mxHosts) == 0 {
				mxHosts = []string{domain.mxHost}
			}
			// RFC 7505: a single "." MX means the domain accepts no mail.
			if len(mxHosts) == 1 && mxHosts[0] == "" {
				recordFailure(fmt.Errorf("domain %q publishes a null MX and does not accept mail", domain.domain))
				return
			}

			// Try the MX hosts one by one in priority order (RFC 5321 §5.1)
			var lastErr error
			for i, mxHost := range mxHosts {
				if i > 0 {
					log.WithContext(ctx).Infof("MX %s delivery failed, trying the next MX: %s (%d/%d)", mxHosts[i-1], mxHost, i+1, len(mxHosts))
				}

				err := tryDeliverToMX(ctx, mxHost, domain.domain, from, fromDomain, buildAddress(tos), data)
				if err == nil {
					return
				}
				lastErr = err

				// A 5xx permanent error is a rejection at the recipient/mailbox level, another MX will not help
				if isPermanentSMTPResponse(err) {
					recordFailure(err)
					return
				}
			}

			recordFailure(lastErr)
		}, nil)
	}
	as.Wait()

	orgMap := map[string]error{}
	errMap.Range(func(key, value any) bool {
		if value != nil {
			orgMap[key.(string)] = value.(error)
		} else {
			orgMap[key.(string)] = nil
		}

		return true
	})

	if len(errEmailAddress) > 0 {
		return errors.New("Delivery failed for the following recipients: " + array.Join(errEmailAddress, ",")), orgMap
	}
	return nil, orgMap
}

func isPermanentSMTPResponse(err error) bool {
	var protocolErr *textproto.Error
	return errors.As(err, &protocolErr) && protocolErr.Code >= 500 && protocolErr.Code <= 599
}

// tryDeliverToMX tries to deliver to a single MX host, following the original strategy order
// STARTTLS:25 → 587 → 465 → plaintext:25
func tryDeliverToMX(ctx *context.Context, mxHost, domain, from, fromDomain string, to []string, data []byte) error {
	mxHost = strings.TrimSuffix(strings.TrimSpace(mxHost), ".")
	if mxHost == "" {
		return fmt.Errorf("domain %q publishes a null MX and does not accept mail", domain)
	}
	// Verify the MX certificate against the MX hostname. Never fall back to an
	// unauthenticated TLS connection or plaintext because a certificate failed.
	err := smtp.SendMail(mxHost, smtp.Address(mxHost, 25), nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	if isPermanentSMTPResponse(err) {
		return err
	}
	log.WithContext(ctx).Infof("SMTP STARTTLS on 25 Send Error. %s", err.Error())

	// Port 587 is submission with STARTTLS, not implicit TLS.
	err = smtp.SendMail(mxHost, smtp.Address(mxHost, 587), nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	if isPermanentSMTPResponse(err) {
		return err
	}
	log.WithContext(ctx).Infof("SMTP STARTTLS on 587 Send Error. %s", err.Error())

	// Port 465 is implicit TLS.
	err = smtp.SendMailWithTls(mxHost, smtp.Address(mxHost, 465), nil, from, fromDomain, to, data)
	if err == nil {
		return nil
	}
	return err
}

func deliveryFailureCause(mxLookupErr, fallbackErr error) error {
	if fallbackErr == nil || isPermanentSMTPResponse(fallbackErr) {
		return fallbackErr
	}

	var lookupDNSErr *net.DNSError
	if !errors.As(mxLookupErr, &lookupDNSErr) || (!lookupDNSErr.IsTimeout && !lookupDNSErr.IsTemporary) {
		return fallbackErr
	}

	var networkErr net.Error
	if !errors.As(fallbackErr, &networkErr) {
		return fallbackErr
	}

	return &temporaryMXFallbackError{
		lookupErr:   mxLookupErr,
		fallbackErr: fallbackErr,
	}
}

func buildAddress(u []*parsemail.User) []string {
	var ret []string

	for _, user := range u {
		ret = append(ret, user.EmailAddress)

	}

	return ret
}
