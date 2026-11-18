package controllers

import (
	"github.com/noelbennettz/imail/utils/context"
	"net/http"
)

type HandlerFunc func(*context.Context, http.ResponseWriter, *http.Request)
