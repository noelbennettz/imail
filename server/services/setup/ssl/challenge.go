package ssl

import (
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

type authInfo struct {
	Domain  string
	Token   string
	KeyAuth string
}

type HttpChallenge struct {
	mu       sync.RWMutex
	AuthInfo map[string]*authInfo
}

var (
	instance        *HttpChallenge
	challengeInitMu sync.Mutex
)

func (h *HttpChallenge) Present(domain, token, keyAuth string) error {
	h.mu.Lock()
	h.AuthInfo[token] = &authInfo{Domain: domain, Token: token, KeyAuth: keyAuth}
	h.mu.Unlock()
	return nil
}

func (h *HttpChallenge) CleanUp(domain, token, keyAuth string) error {
	h.mu.Lock()
	delete(h.AuthInfo, token)
	h.mu.Unlock()
	return nil
}

// GetKeyAuth returns the HTTP-01 response for the token, if present.
func (h *HttpChallenge) GetKeyAuth(token string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	info, ok := h.AuthInfo[token]
	if !ok {
		return "", false
	}
	return info.KeyAuth, true
}

func GetHttpChallengeInstance() *HttpChallenge {
	challengeInitMu.Lock()
	defer challengeInitMu.Unlock()
	if instance == nil {
		instance = &HttpChallenge{AuthInfo: map[string]*authInfo{}}
	}
	return instance
}

type DNSChallenge struct {
	mu       sync.RWMutex
	AuthInfo map[string]*authInfo
}

var dnsInstance *DNSChallenge

func GetDnsChallengeInstance() *DNSChallenge {
	challengeInitMu.Lock()
	defer challengeInitMu.Unlock()
	if dnsInstance == nil {
		dnsInstance = &DNSChallenge{AuthInfo: map[string]*authInfo{}}
	}
	return dnsInstance
}

func (h *DNSChallenge) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	log.Infof("Presenting challenge Info : %+v", info)
	h.mu.Lock()
	h.AuthInfo[token] = &authInfo{Domain: info.FQDN, Token: token, KeyAuth: info.Value}
	h.mu.Unlock()
	return nil
}

func (h *DNSChallenge) CleanUp(domain, token, keyAuth string) error {
	h.mu.Lock()
	delete(h.AuthInfo, token)
	h.mu.Unlock()
	return nil
}

func (h *DNSChallenge) Timeout() (timeout, interval time.Duration) {
	return 60 * time.Minute, 5 * time.Second
}

type DNSItem struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
	Tips  string `json:"tips"`
}

func (h *DNSChallenge) GetDNSSettings(ctx *context.Context) []*DNSItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ret := []*DNSItem{}
	for _, info := range h.AuthInfo {
		ret = append(ret, &DNSItem{
			Type:  "TXT",
			Host:  info.Domain,
			Value: info.KeyAuth,
			TTL:   3600,
		})
	}

	return ret
}
