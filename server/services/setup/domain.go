package setup

import (
	"regexp"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/errors"
	"strings"
)

func GetDomainSettings() (string, string, []string, error) {
	configData, err := config.ReadConfig()
	if err != nil {
		return "", "", []string{}, errors.Wrap(err)
	}

	return configData.Domain, configData.WebDomain, array.Difference(configData.Domains, []string{configData.Domain}), nil
}

var setupDomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

func SetDomainSettings(smtpDomain, webDomain, multiDomains string) error {
	configData, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}

	smtpDomain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(smtpDomain), ".")))
	webDomain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(webDomain), ".")))

	if !setupDomainPattern.MatchString(smtpDomain) {
		return errors.New("invalid smtp domain")
	}

	if !setupDomainPattern.MatchString(webDomain) {
		return errors.New("invalid web domain")
	}

	configData.Domains = []string{}
	seen := map[string]bool{}
	if multiDomains != "" {
		for _, raw := range strings.Split(multiDomains, ",") {
			domain := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ".")))
			if domain == "" || seen[domain] {
				continue
			}
			if !setupDomainPattern.MatchString(domain) {
				return errors.New("invalid multi domain")
			}
			seen[domain] = true
			configData.Domains = append(configData.Domains, domain)
		}
	}

	if !seen[smtpDomain] {
		configData.Domains = append([]string{smtpDomain}, configData.Domains...)
	}

	configData.Domain = smtpDomain
	configData.WebDomain = webDomain

	// Check whether the domain points to this machine todo

	err = config.WriteConfig(configData)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
