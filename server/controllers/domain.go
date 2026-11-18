package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	setupservice "github.com/noelbennettz/imail/services/setup"
	sslmgr "github.com/noelbennettz/imail/services/setup/ssl"
	"github.com/noelbennettz/imail/signal"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"
)

var (
	domainMu      sync.Mutex
	domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

type domainView struct {
	Domain  string `json:"domain"`
	Primary bool   `json:"primary"`
	Users   int64  `json:"users"`
}

type domainCount struct {
	Domain string `xorm:"domain"`
	Count  int64  `xorm:"count"`
}

func DomainList(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	var counts []domainCount
	_ = db.Instance.Table(&models.User{}).Select("domain, count(*) as count").GroupBy("domain").Find(&counts)
	byDomain := make(map[string]int64, len(counts))
	for _, item := range counts {
		byDomain[item.Domain] = item.Count
	}

	list := make([]domainView, 0, len(config.Instance.Domains))
	for _, domain := range config.Instance.Domains {
		list = append(list, domainView{
			Domain:  domain,
			Primary: strings.EqualFold(domain, config.Instance.Domain),
			Users:   byDomain[domain],
		})
	}
	response.NewSuccessResponse(map[string]any{
		"list":    list,
		"primary": config.Instance.Domain,
	}).FPrint(w)
}

func AddDomain(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	domain, err := decodeDomain(req)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}

	domainMu.Lock()
	defer domainMu.Unlock()

	cfg, err := config.ReadConfig()
	if err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to read configuration", "").FPrint(w)
		return
	}
	for _, existing := range cfg.Domains {
		if strings.EqualFold(existing, domain) {
			response.NewErrorResponse(response.ParamsError, "Domain already exists", "").FPrint(w)
			return
		}
	}
	cfg.Domains = append(cfg.Domains, domain)
	if err := config.WriteConfig(cfg); err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to save configuration", "").FPrint(w)
		return
	}
	config.Instance.Domains = cfg.Domains
	log.Infof("Domain added: %s", domain)
	response.NewSuccessResponse("Domain added").FPrint(w)
	scheduleRestart()
}

func DelDomain(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	domain, err := decodeDomain(req)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}

	domainMu.Lock()
	defer domainMu.Unlock()

	cfg, err := config.ReadConfig()
	if err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to read configuration", "").FPrint(w)
		return
	}
	if strings.EqualFold(domain, cfg.Domain) {
		response.NewErrorResponse(response.ParamsError, "The primary domain cannot be removed", "").FPrint(w)
		return
	}
	var users int64
	if count, err := db.Instance.Table(&models.User{}).Where("LOWER(domain) = ?", domain).Count(); err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	} else {
		users = count
	}
	if users > 0 {
		response.NewErrorResponse(response.ParamsError, "Move or delete the mailboxes on this domain first", "").FPrint(w)
		return
	}

	kept := make([]string, 0, len(cfg.Domains))
	found := false
	for _, existing := range cfg.Domains {
		if strings.EqualFold(existing, domain) {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	if !found {
		response.NewErrorResponse(response.ParamsError, "Domain not found", "").FPrint(w)
		return
	}
	if len(kept) == 0 {
		response.NewErrorResponse(response.ParamsError, "At least one domain is required", "").FPrint(w)
		return
	}
	cfg.Domains = kept
	if err := config.WriteConfig(cfg); err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to save configuration", "").FPrint(w)
		return
	}
	config.Instance.Domains = kept
	log.Infof("Domain removed: %s", domain)
	response.NewSuccessResponse("Domain removed").FPrint(w)
	scheduleRestart()
}

func DomainDNSRecords(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	domain, err := decodeDomain(req)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	found := false
	for _, configured := range config.Instance.Domains {
		if strings.EqualFold(configured, domain) {
			found = true
			break
		}
	}
	if !found {
		response.NewErrorResponse(response.ParamsError, "Domain not found", "").FPrint(w)
		return
	}
	records, err := setupservice.GetDNSSettings(ctx)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}
	items := records[domain]
	if items == nil {
		items = []*setupservice.DNSItem{}
	}
	response.NewSuccessResponse(map[string]any{
		"domain":  domain,
		"records": items,
	}).FPrint(w)
}

func SSLInfo(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	var payload struct {
		Domain string `json:"domain"`
	}
	_ = json.NewDecoder(req.Body).Decode(&payload)
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(payload.Domain), "."))
	if domain == "" || !domainPattern.MatchString(domain) {
		domain = config.Instance.Domain
	}
	response.NewSuccessResponse(map[string]any{
		"certificate": sslmgr.GetDomainCertificateInfo(domain),
		"status":      sslmgr.GetStatus(),
	}).FPrint(w)
}

func SSLIssue(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	var payload struct {
		Domain string `json:"domain"`
		Mode   string `json:"mode"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(payload.Domain), "."))
	if domain == "" {
		response.NewErrorResponse(response.ParamsError, "Domain is required", "").FPrint(w)
		return
	}
	provider := "manual"
	if payload.Mode == "dns-cloudflare" {
		provider = "cloudflare"
	}
	if err := sslmgr.IssueForDomain(domain, payload.Mode, provider, strings.TrimSpace(payload.Token)); err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}
	response.NewSuccessResponse("Certificate request started").FPrint(w)
}

func SSLChallengeRecords(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	records := sslmgr.GetDnsChallengeInstance().GetDNSSettings(ctx)
	response.NewSuccessResponse(records).FPrint(w)
}

func SSLUpload(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	var payload struct {
		Domain string `json:"domain"`
		Key    string `json:"key"`
		Cert   string `json:"cert"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(payload.Domain), "."))
	if domain == "" {
		response.NewErrorResponse(response.ParamsError, "Domain is required", "").FPrint(w)
		return
	}
	if err := sslmgr.UploadDomainCertificate(domain, payload.Key, payload.Cert); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	response.NewSuccessResponse("Certificate uploaded").FPrint(w)
	scheduleRestart()
}

func RestartServer(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	response.NewSuccessResponse("Restarting").FPrint(w)
	scheduleRestart()
}

func decodeDomain(req *http.Request) (string, error) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return "", errors.Wrap(err)
	}
	var payload struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", errors.Wrap(err)
	}
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(payload.Domain), "."))
	if !domainPattern.MatchString(domain) {
		return "", errors.New("Enter a valid domain name, for example mail.example.com")
	}
	return domain, nil
}

// scheduleRestart reloads listeners so the TLS SNI allow-list includes the
// new domain. The API response is flushed before the restart begins.
func scheduleRestart() {
	go func() {
		time.Sleep(2 * time.Second)
		signal.RestartChan <- true
	}()
}
