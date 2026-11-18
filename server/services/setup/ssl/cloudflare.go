package ssl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
)

// cloudflareProvider creates and removes the ACME TXT records through the
// Cloudflare API when the operator supplies an API token.
type cloudflareProvider struct {
	token   string
	client  *http.Client
	mu      sync.Mutex
	records map[string]string // fqdn -> record id
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ID string `json:"id"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func NewCloudflareProvider(token string) *cloudflareProvider {
	return &cloudflareProvider{
		token:   token,
		client:  &http.Client{Timeout: 20 * time.Second},
		records: map[string]string{},
	}
}

func (p *cloudflareProvider) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	name := strings.TrimSuffix(info.FQDN, ".")
	zoneID, err := p.findZone(name)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"type":    "TXT",
		"name":    name,
		"content": info.Value,
		"ttl":     120,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result cfResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if !result.Success || len(result.Result) == 0 {
		return fmt.Errorf("cloudflare create TXT failed: %s", strings.TrimSpace(string(raw)))
	}
	p.mu.Lock()
	p.records[name] = result.Result[0].ID
	p.mu.Unlock()
	return nil
}

func (p *cloudflareProvider) CleanUp(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	name := strings.TrimSuffix(info.FQDN, ".")
	p.mu.Lock()
	recordID := p.records[name]
	delete(p.records, name)
	p.mu.Unlock()
	if recordID == "" {
		return nil
	}
	zoneID, err := p.findZone(name)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records/"+recordID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (p *cloudflareProvider) findZone(fqdn string) (string, error) {
	labels := strings.Split(fqdn, ".")
	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		req, err := http.NewRequest(http.MethodGet, "https://api.cloudflare.com/client/v4/zones?name="+candidate, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+p.token)
		resp, err := p.client.Do(req)
		if err != nil {
			return "", err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var zones struct {
			Success bool     `json:"success"`
			Result  []cfZone `json:"result"`
		}
		if err := json.Unmarshal(raw, &zones); err != nil {
			return "", err
		}
		if zones.Success && len(zones.Result) > 0 {
			return zones.Result[0].ID, nil
		}
	}
	return "", fmt.Errorf("cloudflare zone not found for %s (check the API token permissions)", fqdn)
}
