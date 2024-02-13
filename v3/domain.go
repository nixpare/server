package server

import (
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3/life"
)

// Domain rapresents a website domain with all its
// subdomains. It's possible to set:
//   - a function that will be executed (in case there are no
//     errors) before every other logic
//   - global headers, that will be applied in every connection
//   - an error template, that will be used in case your logic
//     will throw any error, so you will have a constant look
type Domain struct {
	name        string
	srv         *HTTPServer
	subdomains  map[string]*Subdomain
	errTemplate *template.Template
	middlewares []MiddlewareFunc
}

type InitCloseFunc func(srv *HTTPServer, d *Domain, sd *Subdomain) error

// Subdomain rapresents a particular subdomain in a domain with all the
// logic. It's required a serve function, which will determine the logic
// of the website, and a Website, with all its options.
// It's possible to set:
//   - default headers, that will be applied in every connection
//   - an error template, that will be used in case your logic
//     will throw any error, so you will have a constant look
//   - the subdomain offline state
//   - an initializer function, called when the server is starting up
//   - a cleanup function, called when the server is shutting down
type Subdomain struct {
	name        string
	Handler     http.Handler
	InitF       InitCloseFunc
	CloseF      InitCloseFunc
	errTemplate *template.Template
	online      bool
	state       *life.LifeCycle
	middlewares []MiddlewareFunc
}

// RegisterDomain registers a domain in the server. It's asked to specify a
// display name used in the logs and the effective URL of the domain (do
// not specify any protocol or port). If the domain name is an empy string
// it will be treated as the default domain (see srv.RegisterDefaultDomain)
func (srv *HTTPServer) RegisterDomain(domain string) (*Domain, error) {
	if _, ok := srv.domains[domain]; ok {
		return nil, fmt.Errorf("domain %s: %w", domain, ErrAlreadyRegistered)
	}

	d := &Domain{
		name:       domain,
		srv:        srv,
		subdomains: make(map[string]*Subdomain),
	}
	d.RegisterSubdomain("*.", HandlerFunc(func (api *API, w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		api.Handler().Error(w, http.StatusNotFound, fmt.Sprintf("Host %s not served by this server", host))
	}))

	srv.domains[domain] = d
	return d, nil
}

// Domain returns the domain with the given name registered in the server, if found
func (srv *HTTPServer) Domain(domain string) *Domain {
	return srv.domains[domain]
}

// DefaultDomain returns the default domain
func (srv *HTTPServer) DefaultDomain() *Domain {
	return srv.domains["*"]
}

func (srv *HTTPServer) RegisterDefault(handler http.Handler) {
	srv.DefaultDomain().DefaultSubdomain().Handler = handler
}

// RegisterSubdomain registers a subdomain in the domain. It's asked to specify the
// subdomain name (with or without trailing dot) and its configuration. It the Website
// Dir field is empty it will be used the default value of "<srv.Path>/public",
// instead if it's not absolute it will be relative to the srv.Path
func (d *Domain) RegisterSubdomain(subdomain string, handler http.Handler) (*Subdomain, error) {
	subdomain = prepSubdomainName(subdomain)
	return d.registerSubdomain(subdomain, handler)
}

func (d *Domain) registerSubdomain(subdomain string, handler http.Handler) (*Subdomain, error) {
	if _, ok := d.subdomains[subdomain]; ok {
		return nil, fmt.Errorf("subdomain: %w", ErrAlreadyRegistered)
	}

	sd := &Subdomain{
		name:    subdomain,
		Handler: handler,
		state:   life.NewLifeCycleState(),
	}
	d.subdomains[subdomain] = sd

	if d.srv.state.GetState() == life.LCS_STARTED {
		sd.start(d.srv, d)
	}

	return sd, nil
}

// Subdomain returns the subdomain with the given name, if found
func (d *Domain) Subdomain(name string) *Subdomain {
	return d.subdomains[prepSubdomainName(name)]
}

// DefaultSubdomain returns the default subdomain, if set
func (d *Domain) DefaultSubdomain() *Subdomain {
	return d.subdomains["*."]
}

func (d *Domain) Name() string {
	return d.name
}

func (d *Domain) AddMiddleware(mw func(next http.Handler) http.Handler) {
	d.middlewares = append(d.middlewares, mw)
}

func (sd *Subdomain) Name() string {
	return sd.name
}

func (sd *Subdomain) AddMiddleware(mw func(next http.Handler) http.Handler) {
	sd.middlewares = append(sd.middlewares, mw)
}

func (sd *Subdomain) start(srv *HTTPServer, d *Domain) {
	if sd.state.AlreadyStarted() {
		return
	}
	sd.state.SetState(life.LCS_STARTING)

	if sd.InitF != nil {
		l := srv.Logger.Clone(nil, true, d.name, sd.name)
		l.Printf(logger.LOG_LEVEL_INFO, "Website %s%s initialization started", sd.name, d.name)

		err := logger.PanicToErr(func() error {
			return sd.InitF(srv, d, sd)
		})
		if err != nil {
			l.Printf(logger.LOG_LEVEL_FATAL, "Website %s%s initialization failed: %v", sd.name, d.name, err)
			sd.state.SetState(life.LCS_STOPPED)
			sd.Disable()
			return
		}

		l.Printf(logger.LOG_LEVEL_INFO, "Website %s%s initialization successful", sd.name, d.name)
	}

	sd.state.SetState(life.LCS_STARTED)
	sd.Enable()
}

func (sd *Subdomain) stop(srv *HTTPServer, d *Domain) {
	if sd.state.AlreadyStopped() {
		return
	}
	sd.state.SetState(life.LCS_STOPPING)

	if sd.CloseF != nil {
		l := srv.Logger.Clone(nil, true, d.name, sd.name)
		l.Printf(logger.LOG_LEVEL_INFO, "Website %s%s cleanup started", sd.name, d.name)

		err := logger.PanicToErr(func() error {
			return sd.CloseF(srv, d, sd)
		})
		if err != nil {
			l.Printf(logger.LOG_LEVEL_FATAL, "Website %s%s cleanup failed: %v", sd.name, d.name, err)
			sd.state.SetState(life.LCS_STOPPED)
			sd.Disable()
			return
		}

		l.Printf(logger.LOG_LEVEL_INFO, "Website %s%s cleanup successful", sd.name, d.name)
	}

	sd.Disable()
	sd.state.SetState(life.LCS_STOPPED)
}

// Enable sets the subdomain to online state
func (sd *Subdomain) Enable() error {
	if sd.state.GetState() == life.LCS_STARTED {
		sd.online = true
		return nil
	}

	return errors.New("can't enable a stopped subdomain")
}

// Disable sets the subdomain to offline state
func (sd *Subdomain) Disable() {
	sd.online = false
}

func (sd *Subdomain) Online() bool {
	return sd.online
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an HTML that contains two specific fields, a .Code one and
// a .Message one, for example like so:
//
//	<h2>Error {{ .Code }}</h2>
//	<p>{{ .Message }}</p>
func (srv *HTTPServer) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	srv.errTemplate = t
	return nil
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an HTML that contains two specific fields, a .Code one and
// a .Message one, for example like so:
//
//	<h2>Error {{ .Code }}</h2>
//	<p>{{ .Message }}</p>
func (d *Domain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	d.errTemplate = t
	return nil
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an HTML that contains two specific fields, a .Code one and
// a .Message one, for example like so:
//
//	<h2>Error {{ .Code }}</h2>
//	<p>{{ .Message }}</p>
func (sd *Subdomain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	sd.errTemplate = t
	return nil
}

// prepSubdomainName sanitizes the subdomain name
func prepSubdomainName(name string) string {
	if name != "" && name != "*" && !strings.HasSuffix(name, ".") {
		name += "."
	}

	return name
}
