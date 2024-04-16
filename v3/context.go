package server

import (
	"net/http"

	"github.com/nixpare/logger/v2"
)

type handler_ctx_key_t string
const handler_ctx_key handler_ctx_key_t = "github.com/nixpare/server/v3.Handler"

type MiddlewareFunc func(next http.Handler) http.Handler

func HandlerFunc(h func(api *Handler, w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := GetHandlerFromCTX(r)
		h(api, w, r)
	})
}

func GetHandlerFromCTX(r *http.Request) *Handler {
	a := r.Context().Value(handler_ctx_key)
	if a == nil {
		return nil
	}
	h, ok := a.(*Handler)
	if !ok {
		return nil
	}

	return h
}

func (h *Handler) Router() *Router {
	return h.router
}

func (h *Handler) Server() *ServerHandler {
	return h.srv
}

func (h *Handler) Domain() *Domain {
	return h.domain
}

func (h *Handler) Subdomain() *Subdomain {
	return h.subdomain
}

func (h *Handler) Logger() logger.Logger {
	return h.l
}
