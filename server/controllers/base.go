package controllers

import (
	"net/http"

	"github.com/noelbennettz/imail/utils/context"
)

type HandlerFunc func(*context.Context, http.ResponseWriter, *http.Request)
