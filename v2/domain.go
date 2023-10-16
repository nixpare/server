package server

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/nixpare/logger/v2"
)

// Domain rapresents a website domain with all its
// subdomains. It's possible to set:
//   - a function that will be executed (in case there are no
//     errors) before every other logic
//   - global headers, that will be applied in every connection
//   - an error template, that will be used in case your logic
//     will throw any error, so you will have a constant look
type Domain struct {
	Name        string
	subdomains  map[string]*Subdomain
	srv         *HTTPServer
	headers     http.Header
	errTemplate *template.Template
	// BeforeServeF sets a function that will be executed before every connection.
	// If this function returns true, the serve function of the subdomain will not be
	// executed
	BeforeServeF BeforeServeFunction
}

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
	Name        string
	Website     *Website
	serveF      ServeFunction
	initF       InitCloseFunction
	closeF      InitCloseFunction
	headers     http.Header
	errTemplate *template.Template
	online      bool
	state       *LifeCycle
}

// SubdomainConfig is used to create a Subdomain. The Website should not be
// an empty struct and if ServeF is not set the server will serve every content
// inside the Website.Dir folder (see Route.StaticServe(true) for the logic and
// Domain.RegisterSubdomain for the folder behaviour), however InitF and CloseF
// are optional
type SubdomainConfig struct {
	// Website will be copied to the subdomain and later will be
	// linked in every connection
	Website Website
	// ServeF is the function holding the logic behind the website
	ServeF ServeFunction
	// InitF is the function called upon server startup
	InitF InitCloseFunction
	// CloseF is the function called upon server shutdown
	CloseF InitCloseFunction
}

// RegisterDomain registers a domain in the server. It's asked to specify a
// display name used in the logs and the effective URL of the domain (do
// not specify any protocol or port). If the domain name is an empy string
// it will be treated as the default domain (see srv.RegisterDefaultDomain)
func (srv *HTTPServer) RegisterDomain(displayName, domain string) (*Domain, error) {
	if _, ok := srv.domains[domain]; ok {
		return nil, fmt.Errorf("domain: %w", ErrAlreadyRegistered)
	}
	
	d := &Domain{
		Name:       displayName,
		subdomains: make(map[string]*Subdomain),
		srv:        srv,
		headers:    make(http.Header),
	}

	srv.domains[domain] = d
	return d, nil
}

// RegisterDefaultDomain registers a domain that is called if no other domain
// matches perfectly the incoming connection
func (srv *HTTPServer) RegisterDefaultDomain(displayName string) (*Domain, error) {
	return srv.RegisterDomain(displayName, "")
}

// Domain returns the domain with the given name registered in the server, if found
func (srv *HTTPServer) Domain(domain string) *Domain {
	return srv.domains[domain]
}

// DefaultDomain returns the default domain, if set
func (srv *HTTPServer) DefaultDomain() *Domain {
	return srv.domains[""]
}

// RegisterDefaultRoute is a shortcut for registering the default logic applied for every
// connection not matching any other specific domain and subdomain. It's
// the combination of srv.RegisterDefaultDomain(displayName).RegisterDefaultSubdomain(c)
func (srv *HTTPServer) RegisterDefaultRoute(displayName string, c SubdomainConfig) (*Domain, *Subdomain, error) {
	d, err := srv.RegisterDefaultDomain(displayName)
	if err != nil {
		return d, nil, err
	}

	sd, err := d.RegisterDefaultSubdomain(c)
	return d, sd, err
}

// RegisterSubdomain registers a subdomain in the domain. It's asked to specify the
// subdomain name (with or without trailing dot) and its configuration. It the Website
// Dir field is empty it will be used the default value of "<srv.Path>/public",
// instead if it's not absolute it will be relative to the srv.Path
func (d *Domain) RegisterSubdomain(subdomain string, c SubdomainConfig) (*Subdomain, error) {
	subdomain = prepSubdomainName(subdomain)
	if _, ok := d.subdomains[subdomain]; ok {
		return nil, fmt.Errorf("subdomain: %w", ErrAlreadyRegistered)
	}

	if c.ServeF == nil {
		c.Website.AllFolders = []string{""}
		c.ServeF = func(route *Route) { route.StaticServe(true) }
	}

	if !isAbs(c.Website.Dir) {
		if c.Website.Dir == "" {
			c.Website.Dir = d.srv.ServerPath + "/public"
		} else {
			c.Website.Dir = d.srv.ServerPath + "/" + c.Website.Dir
		}
	} else {
		if strings.HasPrefix(c.Website.Dir, "~") {
			home, err := os.UserHomeDir()
			if err == nil {
				c.Website.Dir = strings.Replace(c.Website.Dir, "~", home, 1)
			}
		}
	}

	ws := &Website{
		Name:                   c.Website.Name,
		Dir:                    c.Website.Dir,
		MainPages:              c.Website.MainPages,
		NoLogPages:             c.Website.NoLogPages,
		AllFolders:             c.Website.AllFolders,
		HiddenFolders:          c.Website.HiddenFolders,
		PageHeaders:            c.Website.PageHeaders,
		XFiles:                 make(map[string]string),
		AvoidMetricsAndLogging: c.Website.AvoidMetricsAndLogging,
	}

	for key, value := range c.Website.XFiles {
		if value == "" {
			ws.XFiles[key] = key
		}

		ws.XFiles[key] = value
	}

	sd := &Subdomain{
		Name: subdomain, Website: ws,
		serveF: c.ServeF, initF: c.InitF, closeF: c.CloseF,
		headers: make(http.Header),
		state:   NewLifeCycleState(),
	}
	d.subdomains[subdomain] = sd

	if d.srv.state.GetState() == LCS_STARTED {
		sd.start(d.srv, d)
	}

	return sd, nil
}

// RegisterDefaultSubdomain registers a subdomain that is called if no other one
// matches perfectly the incoming connection for the same domain
func (d *Domain) RegisterDefaultSubdomain(c SubdomainConfig) (*Subdomain, error) {
	return d.RegisterSubdomain("*", c)
}

// Subdomain returns the subdomain with the given name, if found
func (d *Domain) Subdomain(name string) *Subdomain {
	return d.subdomains[prepSubdomainName(name)]
}

// DefaultSubdomain returns the default subdomain, if set
func (d *Domain) DefaultSubdomain() *Subdomain {
	return d.subdomains["*"]
}

// SetHeader adds a header to the collection of headers used in every connection
func (d *Domain) SetHeader(name, value string) {
	d.headers.Set(name, value)
}

// SetHeaders adds headers to the collection of headers used in every connection.
// This is a faster way to set multiple headers at the same time, instead of using
// domain.SetHeader. The headers must be provided in this way:
//
//	headers := [][2]string {
//		{ "name1", "value1" },
//		{ "name2", "value2" },
//	}
//	d.SetHeaders(headers)
func (d *Domain) SetHeaders(headers [][2]string) {
	for _, header := range headers {
		d.SetHeader(header[0], header[1])
	}
}

// RemoveHeader removes a header with the given name
func (d *Domain) RemoveHeader(name string) {
	d.headers.Del(name)
}

// Headers returns the default headers of the domain
func (d *Domain) Headers() http.Header {
	return d.headers
}

// EnableSubdomain sets a subdomain to online state
func (d *Domain) EnableSubdomain(name string) error {
	sd := d.Subdomain(name)
	if sd == nil {
		return fmt.Errorf("subdomain: %w", ErrNotFound)
	}

	return sd.Enable()
}

// DisableSubdomain sets a subdomain to offline state
func (d *Domain) DisableSubdomain(name string) error {
	sd := d.Subdomain(name)
	if sd == nil {
		return fmt.Errorf("subdomain: %w", ErrNotFound)
	}

	sd.Disable()
	return nil
}

// SetHeader adds a header to the collection of headers used in every connection
func (sd *Subdomain) SetHeader(name, value string) {
	sd.headers.Set(name, value)
}

// SetHeaders adds headers to the collection of headers used in every connection.
// This is a faster way to set multiple headers at the same time, instead of using
// subdomain.SetHeader. The headers must be provided in this way:
//
//	headers := [][2]string {
//		{ "name1", "value1" },
//		{ "name2", "value2" },
//	}
//	d.SetHeaders(headers)
func (sd *Subdomain) SetHeaders(headers [][2]string) {
	for _, header := range headers {
		sd.SetHeader(header[0], header[1])
	}
}

// RemoveHeader removes a header with the given name
func (sd *Subdomain) RemoveHeader(name string) {
	sd.headers.Del(name)
}

// Header returns the default headers
func (sd *Subdomain) Header() http.Header {
	return sd.headers
}

func (sd *Subdomain) start(srv *HTTPServer, d *Domain) {
	if sd.state.AlreadyStarted() {
		return
	}
	sd.state.SetState(LCS_STARTING)

	if sd.initF != nil {
		l := srv.Logger.Clone(nil, d.Name, strings.TrimRight(sd.Name, "."), sd.Website.Name)
		l.Printf(logger.LOG_LEVEL_INFO, "Website %s (%s) initialization started", sd.Website.Name, d.Name)

		err := logger.PanicToErr(func() error {
			return sd.initF(srv, d, sd)
		})
		if err != nil {
			l.Printf(logger.LOG_LEVEL_FATAL, "Website %s (%s) initialization failed: %v", sd.Website.Name, d.Name, err)
			sd.state.SetState(LCS_STOPPED)
			sd.Disable()
			return
		}

		l.Printf(logger.LOG_LEVEL_INFO, "Website %s (%s) initialization successful", sd.Website.Name, d.Name)
	}

	sd.state.SetState(LCS_STARTED)
	sd.Enable()
}

func (sd *Subdomain) stop(srv *HTTPServer, d *Domain) {
	if sd.state.AlreadyStopped() {
		return
	}
	sd.state.SetState(LCS_STOPPING)

	if sd.closeF != nil {
		l := srv.Logger.Clone(nil, d.Name, strings.TrimRight(sd.Name, "."), sd.Website.Name)
		l.Printf(logger.LOG_LEVEL_INFO, "Website %s (%s) cleanup started", sd.Website.Name, d.Name)

		err := logger.PanicToErr(func() error {
			return sd.closeF(srv, d, sd)
		})
		if err != nil {
			l.Printf(logger.LOG_LEVEL_FATAL, "Website %s (%s) cleanup failed: %v", sd.Website.Name, d.Name, err)
			sd.state.SetState(LCS_STOPPED)
			sd.Disable()
			return
		}

		l.Printf(logger.LOG_LEVEL_INFO, "Website %s (%s) cleanup successful", sd.Website.Name, d.Name)
	}

	sd.Disable()
	sd.state.SetState(LCS_STOPPED)
}

// Enable sets the subdomain to online state
func (sd *Subdomain) Enable() error {
	if sd.state.GetState() == LCS_STARTED {
		sd.online = true
		return nil
	}

	return errors.New("can't enable a stopped subdomain")
}

// Disable sets the subdomain to offline state
func (sd *Subdomain) Disable() {
	sd.online = false
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
