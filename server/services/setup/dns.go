package setup

import (
	"fmt"
	"strings"

	"github.com/noelbennettz/imail/config"

	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/services/auth"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/ip"
	"github.com/noelbennettz/imail/utils/netutil"
)

type DNSItem struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
	Tips  string `json:"tips"`
}

func webHostLabel(webDomain, primary string) string {
	webDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(webDomain), "."))
	primary = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(primary), "."))
	if webDomain == "" || primary == "" || webDomain == primary {
		return ""
	}
	suffix := "." + primary
	if strings.HasSuffix(webDomain, suffix) {
		return strings.TrimSuffix(webDomain, suffix)
	}
	return ""
}

func GetDNSSettings(ctx *context.Context) (map[string][]*DNSItem, error) {
	configData, err := config.ReadConfig()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	ret := make(map[string][]*DNSItem)

	ipv4 := ip.GetIPv4()
	ipv6 := ip.GetIPv6()
	mode := netutil.EffectiveMode(configData.ListenMode)
	includeV4 := mode != netutil.ModeIPv6
	includeV6 := mode != netutil.ModeIPv4
	hasAddress := false

	for _, domain := range configData.Domains {
		records := []*DNSItem{}
		hosts := []string{"smtp", "imap", "pop", "@"}
		if strings.EqualFold(domain, configData.Domain) {
			if webHost := webHostLabel(configData.WebDomain, configData.Domain); webHost != "" {
				hosts = append(hosts, webHost)
			}
		}
		addHosts := func(recordType, value, tips string) {
			for _, host := range hosts {
				records = append(records, &DNSItem{Type: recordType, Host: host, Value: value, TTL: 3600, Tips: tips})
			}
		}
		if includeV4 && ipv4 != "" {
			addHosts("A", ipv4, i18n.GetText(ctx.Lang, "ip_taps"))
			hasAddress = true
		}
		if includeV6 && ipv6 != "" {
			addHosts("AAAA", ipv6, i18n.GetText(ctx.Lang, "ip_taps"))
			hasAddress = true
		}
		if !hasAddress {
			addHosts("A", "[Your Server IPv4 Address]", i18n.GetText(ctx.Lang, "ip_taps"))
		}
		records = append(records,
			&DNSItem{Type: "MX", Host: "@", Value: fmt.Sprintf("smtp.%s", domain), TTL: 3600},
			&DNSItem{Type: "TXT", Host: "@", Value: "v=spf1 a mx ~all", TTL: 3600},
			&DNSItem{Type: "TXT", Host: "default._domainkey", Value: auth.DkimGen(), TTL: 3600},
			&DNSItem{Type: "TXT", Host: "_dmarc", Value: fmt.Sprintf("v=DMARC1; p=quarantine; rua=mailto:postmaster@%s", domain), TTL: 3600},
			&DNSItem{Type: "PTR", Host: "@", Value: "smtp." + configData.Domain, TTL: 3600, Tips: "Set the reverse DNS (PTR) of your public address to this hostname at your provider."},
		)
		ret[domain] = records
	}

	return ret, nil
}
