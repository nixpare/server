package server

import (
	"net/http"

	"github.com/nixpare/logger/v2"
)

type API interface {
	Header() http.Header
	Write(b []byte) (int, error)
	Router() *Router
	Server() *HTTPServer
	Handler() *Handler
	Domain() *Domain
	Subdomain() *Subdomain
	Logger() logger.Logger
}

func HandlerFunc(h func(api API, w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api := w.(API)
		h(api, w, r)
	})
}

type api struct {
	handler *Handler
	app     http.Handler
	w       http.ResponseWriter
}

func (ah api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ah.w = w
	ah.app.ServeHTTP(ah, r)
}

func (ah api) Header() http.Header {
	return ah.w.Header()
}

func (ah api) Write(b []byte) (int, error) {
	return ah.w.Write(b)
}

func (ah api) WriteHeader(statusCode int) {
	ah.w.WriteHeader(statusCode)
}

func (ah api) Router() *Router {
	return ah.handler.router
}

func (ah api) Server() *HTTPServer {
	return ah.handler.srv
}

func (ah api) Handler() *Handler {
	return ah.handler
}

func (ah api) Domain() *Domain {
	return ah.handler.domain
}

func (ah api) Subdomain() *Subdomain {
	return ah.handler.subdomain
}

func (ah api) Logger() logger.Logger {
	return ah.handler.Logger
}
