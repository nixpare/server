package server

import (
	"net/http"

	"github.com/nixpare/logger/v2"
)

type APICtxKeyT string

const API_CTX_KEY APICtxKeyT = "nix-handler"

type API struct {
	h *Handler
}

func HandlerFunc(h func(api *API, w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := GetAPIFromReq(r)
		h(api, w, r)
	})
}

func GetAPIFromReq(r *http.Request) *API {
	return r.Context().Value(API_CTX_KEY).(*API)
}

func (ah *API) Router() *Router {
	return ah.h.router
}

func (ah *API) Server() *HTTPServer {
	return ah.h.srv
}

func (ah *API) Handler() *Handler {
	return ah.h
}

func (ah *API) Domain() *Domain {
	return ah.h.domain
}

func (ah *API) Subdomain() *Subdomain {
	return ah.h.subdomain
}

func (ah *API) Logger() logger.Logger {
	return ah.h.Logger
}
