package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

// securitySettingsMu serializes read-modify-write cycles of config.json so two
// administrators cannot drop each other's change.
var securitySettingsMu sync.Mutex

type securitySettingsRequest struct {
	// LimitFailedSignins is a pointer so a request that omits the field is
	// rejected instead of silently switching the protection off.
	LimitFailedSignins *bool `json:"limitFailedSignins"`
}

// SecuritySettings reports the live security switches.
func SecuritySettings(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	response.NewSuccessResponse(map[string]any{
		"limitFailedSignins": config.LoginLimiterEnabled(),
	}).FPrint(w)
}

// SaveSecuritySettings persists the security switches and applies them to the
// running process. Neither switch needs a restart: the listeners read the
// limits on every authentication attempt.
func SaveSecuritySettings(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	var payload securitySettingsRequest
	if err := json.Unmarshal(data, &payload); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	if payload.LimitFailedSignins == nil {
		response.NewErrorResponse(response.ParamsError, "limitFailedSignins is required", "").FPrint(w)
		return
	}
	enabled := *payload.LimitFailedSignins

	securitySettingsMu.Lock()
	defer securitySettingsMu.Unlock()

	cfg, err := config.ReadConfig()
	if err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to read configuration", "").FPrint(w)
		return
	}
	cfg.LimitFailedSignins = &enabled
	if err := config.WriteConfig(cfg); err != nil {
		response.NewErrorResponse(response.ServerError, "Unable to save configuration", "").FPrint(w)
		return
	}
	config.Instance.LimitFailedSignins = cfg.LimitFailedSignins
	config.SetLoginLimiterEnabled(enabled)

	log.Infof("Failed sign-in limit set to %t", enabled)
	response.NewSuccessResponse(i18n.GetText(ctx.Lang, "succ")).FPrint(w)
}
