package server

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
)

// Domain rapresents a website domain with all its
// subdomains. It's possible to set:
//  - global headers, that will be applied in every connection
//  - an error template, that will be used in case your logic
//		will throw any error, so you will have a constant look
type Domain struct {
	Name        string
	subdomains  map[string]*Subdomain
	srv         *Server
	headers     http.Header
	errTemplate *template.Template
}

// Subdomain rapresents a particular subdomain in a domain with all the
// logic. It's required a serve function, which will determine the logic
// of the website, and a Website, with all its options.
// It's possible to set:
//  - default headers, that will be applied in every connection
//  - an error template, that will be used in case your logic
//		will throw any error, so you will have a constant look
//	- the subdomain to be offline, can be reverted
//	- an initializer function, called when the server is starting up
//	- a cleanup function, called when the server is shutting down
type Subdomain struct {
	Name        string
	website     *Website
	serveF 		ServeFunction
	initF  		InitCloseFunction
	closeF  	InitCloseFunction
	headers     http.Header
	errTemplate *template.Template
	offline     bool
}

// SubdomainConfig is used to create a Subdomain. The Website and ServeF
// fields must not be nil, instead InitF and CloseF are optional
type SubdomainConfig struct {
	Website Website
	ServeF  ServeFunction
	InitF   InitCloseFunction
	CloseF  InitCloseFunction
}

// RegisterDomain registers a domain in the server. It's asked to specify a
// display name used in the logs and the effective URL of the domain (do
// not specify any protocol or port). If the domain name is an empy string
// it will be treated as the default domain (see srv.RegisterDefaultDomain)
func (srv *Server) RegisterDomain(displayName, domain string) *Domain {
	d := &Domain{
		Name: displayName,
		subdomains: make(map[string]*Subdomain),
		srv: srv,
		headers: make(http.Header),
	}

	srv.domains[domain] = d
	return d
}

// RegisterDefaultDomain registers a domain that is called if no other domain
// matches perfectly the incoming connection
func (srv *Server) RegisterDefaultDomain(displayName string) *Domain {
	return srv.RegisterDomain(displayName, "")
}

// Domain returns the domain with the given name registered in the server, if found
func (srv *Server) Domain(domain string) *Domain {
	return srv.domains[domain]
}

// DefaultDomain returns the default domain, if set
func (srv *Server) DefaultDomain() *Domain {
	return srv.domains[""]
}

// This is a shortcut for registering the default logic applied for every
// connection not matching any other specific domain and subdomain. It's
// the combination of srv.RegisterDefaultDomain(displayName).RegisterDefaultSubdomain(c)
func (srv *Server) RegisterDefaultRoute(displayName string, c SubdomainConfig) (*Domain, *Subdomain) {
	d := srv.RegisterDefaultDomain(displayName)
	sd := d.RegisterDefaultSubdomain(c)

	return d, sd
}

// RegisterSubdomain registers a subdomain in the domain. It's asked to specify the
// subdomain name (with or without trailing dot) and its configuration. It the Website
// Dir field is empty it will be used the default value of "<srv.ServerPath>/public",
// instead if it's not absolute it will be relative to the srv.ServerPath
func (d *Domain) RegisterSubdomain(subdomain string, c SubdomainConfig) *Subdomain {
	subdomain = prepSubdomainName(subdomain)

	if c.ServeF == nil {
		c.Website.AllFolders = []string{""}
		if c.Website.Dir == "" {
			c.Website.Dir = d.srv.ServerPath + "/public"
		}
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

	ws := new(Website)
	*ws = c.Website

	sd := &Subdomain{
		Name: subdomain, website: ws,
		serveF: c.ServeF, initF: c.InitF, closeF: c.CloseF,
		headers: make(http.Header),
	}
	d.subdomains[subdomain] = sd

	return sd
}

// RegisterDefaultSubdomain registers a subdomain that is called if no other one
// matches perfectly the incoming connection for the same domain
func (d *Domain) RegisterDefaultSubdomain(c SubdomainConfig) *Subdomain {
	return d.RegisterSubdomain("*", c)
}

// Subdomain returns the subdomain with the given name, if found
func (d *Domain) Subdomain(subdomain string) *Subdomain {
	return d.subdomains[subdomain]
}

// Subdomain returns the default subdomain, if set
func (d *Domain) DefaultSubdomain() *Subdomain {
	return d.subdomains["*"]
}

// SetHeader adds a header to the collection of headers used in every connection
func (d *Domain) SetHeader(name, value string) *Domain {
	d.headers.Set(name, value)
	return d
}

// SetHeaders adds headers to the collection of headers used in every connection.
// This is a faster way to set multiple headers at the same time, instead of using
// domain.SetHeader. The headers must be provided in this way:
//		headers := [][2]string {
//			{ "name1", "value1" },
//			{ "name2", "value2" },
//		}
//		d.SetHeaders(headers)
func (d *Domain) SetHeaders(headers [][2]string) *Domain {
	for _, header := range headers {
		d.SetHeader(header[0], header[1])
	}
	return d
}

// RemoveHeader removes a header with the given name
func (d *Domain) RemoveHeader(name string) *Domain {
	d.headers.Del(name)
	return d
}

// Header returns the default headers
func (d *Domain) Header() http.Header {
	return d.headers
}

// EnableSubdomain enables a subdomain
func (d *Domain) EnableSubdomain(name string) *Domain {
	name = prepSubdomainName(name)

	sd := d.subdomains[name]
	if sd != nil {
		sd.Enable()
	}

	return d
}

// DisableSubdomain disables a subdomain
func (d *Domain) DisableSubdomain(name string) *Domain {
	name = prepSubdomainName(name)

	sd := d.subdomains[name]
	if sd != nil {
		sd.Disable()
	}

	return d
}

// RemoveSubdomain unregisters a subdomain
func (d *Domain) RemoveSubdomain(name string) *Domain {
	delete(d.subdomains, prepSubdomainName(name))
	return d
}

// SetHeader adds a header to the collection of headers used in every connection
func (sd *Subdomain) SetHeader(name, value string) *Subdomain {
	sd.headers.Set(name, value)
	return sd
}

// SetHeaders adds headers to the collection of headers used in every connection.
// This is a faster way to set multiple headers at the same time, instead of using
// subdomain.SetHeader. The headers must be provided in this way:
//		headers := [][2]string {
//			{ "name1", "value1" },
//			{ "name2", "value2" },
//		}
//		d.SetHeaders(headers)
func (sd *Subdomain) SetHeaders(headers [][2]string) *Subdomain {
	for _, header := range headers {
		sd.SetHeader(header[0], header[1])
	}
	return sd
}

// RemoveHeader removes a header with the given name
func (sd *Subdomain) RemoveHeader(name string) *Subdomain {
	sd.headers.Del(name)
	return sd
}

// Header returns the default headers
func (sd *Subdomain) Header() http.Header {
	return sd.headers
}

// Enable sets the subdomain to be online and working
func (sd *Subdomain) Enable() *Subdomain {
	sd.offline = false
	return sd
}

// Disable sets the subdomain to be offline
func (sd *Subdomain) Disable() *Subdomain {
	sd.offline = true
	return sd
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an html that contains two specific fields, a .Code one and
// a .Message one, for example like so:
// 	<h2>Error {{ .Code }}</h2>
// 	<p>{{ .Message }}</p>
func (srv *Server) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	srv.errTemplate = t
	return nil
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an html that contains two specific fields, a .Code one and
// a .Message one, for example like so:
// 	<h2>Error {{ .Code }}</h2>
// 	<p>{{ .Message }}</p>
func (d *Domain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	d.errTemplate = t
	return nil
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an html that contains two specific fields, a .Code one and
// a .Message one, for example like so:
// 	<h2>Error {{ .Code }}</h2>
// 	<p>{{ .Message }}</p>
func (sd *Subdomain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	sd.errTemplate = t
	return nil
}
