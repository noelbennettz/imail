package controllers

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/quota"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

type userCreateRequest struct {
	Id       int    `json:"id"`
	Account  string `json:"account"`
	Username string `json:"username"`
	Password string `json:"password"`
	// Disabled is a pointer so an edit request that omits it keeps the current
	// state instead of silently enabling a disabled mailbox.
	Disabled *int `json:"disabled"`
	// Domain assigns the mailbox to one of the configured domains.
	Domain string `json:"domain"`
	// QuotaMB sets the mailbox limit in MiB. nil keeps the current value on
	// edit and applies the server default on create.
	QuotaMB *int64 `json:"quota_mb"`
}

var mailboxAccountPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,19}$`)

func validMailboxCredentials(account, password string) bool {
	return mailboxAccountPattern.MatchString(account) && len(password) >= 8
}

type userView struct {
	ID        int    `json:"ID"`
	Account   string `json:"Account"`
	Name      string `json:"Name"`
	Domain    string `json:"domain"`
	Disabled  int    `json:"Disabled"`
	IsAdmin   int    `json:"IsAdmin"`
	Quota     int64  `json:"quota"`
	QuotaUsed int64  `json:"quota_used"`
}

func viewOf(user models.User) userView {
	used, _ := quota.UsedBytes(user.ID)
	return userView{
		ID:        user.ID,
		Account:   user.Account,
		Name:      user.Name,
		Domain:    user.Domain,
		Disabled:  user.Disabled,
		IsAdmin:   user.IsAdmin,
		Quota:     user.Quota,
		QuotaUsed: used,
	}
}

func CreateUser(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No Access Privileges", "").FPrint(w)
		return
	}

	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Errorf("%+v", err)
	}
	var reqData userCreateRequest
	err = json.Unmarshal(reqBytes, &reqData)
	if err != nil {
		log.Errorf("%+v", err)
	}

	if reqData.Username == "" || !validMailboxCredentials(reqData.Account, reqData.Password) {
		response.NewErrorResponse(response.ParamsError, "Account must be 1-20 letters/digits/._- and password at least 8 characters", "").FPrint(w)
		return
	}

	var user models.User
	user.Name = reqData.Username
	user.Password = password.Encode(reqData.Password)
	user.Account = reqData.Account
	user.Domain = strings.ToLower(strings.TrimSpace(reqData.Domain))
	if user.Domain == "" {
		user.Domain = config.Instance.Domain
	}
	if !array.InArray(user.Domain, config.Instance.Domains) {
		response.NewErrorResponse(response.ParamsError, "domain is not configured", "").FPrint(w)
		return
	}
	if reqData.QuotaMB != nil {
		user.Quota = *reqData.QuotaMB * 1024 * 1024
	} else {
		user.Quota = config.Instance.DefaultQuotaBytes()
	}

	if found, err := db.Instance.Table("user").Where("account=?", reqData.Account).Exist(); err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	} else if found {
		response.NewErrorResponse(response.ParamsError, "Account already exists", "").FPrint(w)
		return
	}

	_, err = db.Instance.Insert(&user)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}

	response.NewSuccessResponse(viewOf(user)).FPrint(w)
}

type userListRequest struct {
	CurrentPage int    `json:"current_page"`
	PageSize    int    `json:"page_size"`
	Domain      string `json:"domain"`
}

func UserList(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No Access Privileges", "").FPrint(w)
		return
	}

	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Errorf("%+v", err)
	}
	var reqData userListRequest
	err = json.Unmarshal(reqBytes, &reqData)
	if err != nil {
		log.Errorf("%+v", err)
	}

	if reqData.CurrentPage < 1 {
		reqData.CurrentPage = 1
	}
	if reqData.PageSize <= 0 {
		reqData.PageSize = 15
	}
	if reqData.PageSize > 100 {
		reqData.PageSize = 100
	}
	offset := (reqData.CurrentPage - 1) * reqData.PageSize

	var users []models.User

	domain := strings.ToLower(strings.TrimSpace(reqData.Domain))
	if domain == "" {
		domain = strings.ToLower(config.Instance.Domain)
	}
	session := db.Instance.Table(&models.User{}).Where("LOWER(domain) = ?", domain)
	if strings.EqualFold(domain, config.Instance.Domain) {
		// Accounts created before domains were introduced have no domain and
		// belong to the primary mailbox domain.
		session = db.Instance.Table(&models.User{}).Where("(LOWER(domain) = ? OR domain = '')", domain)
	}
	totalNum, err := session.Limit(reqData.PageSize, offset).FindAndCount(&users)
	if err != nil {
		log.Errorf("%+v", err)
	}

	views := make([]userView, 0, len(users))
	for _, user := range users {
		views = append(views, viewOf(user))
	}

	response.NewSuccessResponse(map[string]any{
		"current_page": reqData.CurrentPage,
		"total_page":   cast.ToInt(math.Ceil(cast.ToFloat64(totalNum) / cast.ToFloat64(reqData.PageSize))),
		"list":         views,
	}).FPrint(w)
}

func Info(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	domains := config.Instance.Domains
	domains = array.Difference(domains, []string{config.Instance.Domain})
	domains = append([]string{config.Instance.Domain}, domains...)

	var self models.User
	_, _ = db.Instance.ID(ctx.UserID).Get(&self)
	used, _ := quota.UsedBytes(ctx.UserID)

	response.NewSuccessResponse(map[string]any{
		"account":    ctx.UserAccount,
		"name":       ctx.UserName,
		"is_admin":   ctx.IsAdmin,
		"domains":    domains,
		"domain":     ctx.UserDomain,
		"quota":      self.Quota,
		"quota_used": used,
	}).FPrint(w)
}

func EditUser(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No Access Privileges", "").FPrint(w)
		return
	}

	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Errorf("%+v", err)
	}
	var reqData userCreateRequest
	err = json.Unmarshal(reqBytes, &reqData)
	if err != nil {
		log.Errorf("%+v", err)
	}

	if reqData.Id == 0 && reqData.Account == "" {
		response.NewErrorResponse(response.ParamsError, "Params Error", "").FPrint(w)
		return
	}
	var user models.User
	if reqData.Id != 0 {
		_, err = db.Instance.Where("id=?", reqData.Id).Get(&user)
		if err != nil {
			log.Errorf("SQL Error: %+v", err)
		}
	} else {
		_, err = db.Instance.Where("account=?", reqData.Account).Get(&user)
		if err != nil {
			log.Errorf("SQL Error: %+v", err)
		}
	}
	if user.ID == 0 {
		response.NewErrorResponse(response.ParamsError, "User not found", "").FPrint(w)
		return
	}
	disabled := user.Disabled
	if reqData.Disabled != nil {
		disabled = *reqData.Disabled
		if disabled != 0 {
			disabled = 1
		}
	}
	revokeSessions := reqData.Password != "" || disabled != user.Disabled
	updates := map[string]interface{}{}
	if reqData.Disabled != nil {
		updates["disabled"] = disabled
		if disabled != user.Disabled {
			user.Disabled = disabled
		}
	}
	if reqData.Username != "" && reqData.Username != user.Name {
		user.Name = reqData.Username
		updates["name"] = user.Name
	}
	if reqData.QuotaMB != nil {
		user.Quota = *reqData.QuotaMB * 1024 * 1024
		updates["quota"] = user.Quota
	}
	if domain := strings.ToLower(strings.TrimSpace(reqData.Domain)); domain != "" && domain != user.Domain {
		if !array.InArray(domain, config.Instance.Domains) {
			response.NewErrorResponse(response.ParamsError, "domain is not configured", "").FPrint(w)
			return
		}
		user.Domain = domain
		updates["domain"] = user.Domain
	}

	if reqData.Password != "" {
		if len(reqData.Password) < 8 {
			response.NewErrorResponse(response.ParamsError, "Password must be at least 8 characters", "").FPrint(w)
			return
		}
		user.Password = password.Encode(reqData.Password)
		updates["password"] = user.Password
	}

	if len(updates) == 0 {
		response.NewSuccessResponse(user).FPrint(w)
		return
	}

	update := db.Instance.Table(&models.User{}).ID(user.ID)
	if revokeSessions {
		update.Incr("auth_version")
	}
	num, err := update.Update(updates)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}
	if num == 0 {
		response.NewErrorResponse(response.ServerError, "No Data Update", "").FPrint(w)
		return
	}

	response.NewSuccessResponse(user).FPrint(w)
}
