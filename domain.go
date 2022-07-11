package server

import (
	"fmt"
	"html/template"
	"net/http"
)

type Domain struct {
	Name        string
	subdomains  map[string]*Subdomain
	srv         *Server
	headers     http.Header
	errTemplate *template.Template
}

type Subdomain struct {
	Name        string
	website     *Website
	serveF 		ServeFunction
	initF  		InitFunction
	headers     http.Header
	errTemplate *template.Template
	offline     bool
}

type SubdomainConfig struct {
	Website Website
	ServeF  ServeFunction
	InitF   InitFunction
}

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

func (srv *Server) RegisterDefaultDomain(displayName string) *Domain {
	return srv.RegisterDomain(displayName, "")
}

func (srv *Server) Domain(domain string) *Domain {
	return srv.domains[domain]
}

func (srv *Server) DefaultDomain() *Domain {
	return srv.domains[""]
}

func (srv *Server) RegisterDefaultRoute(displayName string, c SubdomainConfig) (*Domain, *Subdomain) {
	d := srv.RegisterDefaultDomain(displayName)
	sd := d.RegisterDefaultSubdomain(c)

	return d, sd
}

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
		c.Website.Dir = d.srv.ServerPath + "/" + c.Website.Dir
	}

	ws := new(Website)
	*ws = c.Website

	sd := &Subdomain{
		Name: subdomain, website: ws,
		serveF: c.ServeF, initF: c.InitF,
		headers: make(http.Header),
	}
	d.subdomains[subdomain] = sd

	return sd
}

func (d *Domain) RegisterDefaultSubdomain(c SubdomainConfig) *Subdomain {
	return d.RegisterSubdomain("*", c)
}

func (d *Domain) Subdomain(subdomain string) *Subdomain {
	return d.subdomains[subdomain]
}

func (d *Domain) DefaultSubdomain() *Subdomain {
	return d.subdomains["*"]
}

func (d *Domain) SetHeader(name, value string) *Domain {
	d.headers.Set(name, value)
	return d
}

func (d *Domain) SetHeaders(headers [][2]string) *Domain {
	for _, header := range headers {
		d.SetHeader(header[0], header[1])
	}
	return d
}

func (d *Domain) RemoveHeader(name string) *Domain {
	d.headers.Del(name)
	return d
}

func (d *Domain) Header() http.Header {
	return d.headers
}

func (d *Domain) EnableSubdomain(name string) *Domain {
	name = prepSubdomainName(name)

	sd := d.subdomains[name]
	if sd != nil {
		sd.Enable()
	}

	return d
}

func (d *Domain) DisableSubdomain(name string) *Domain {
	name = prepSubdomainName(name)

	sd := d.subdomains[name]
	if sd != nil {
		sd.Disable()
	}

	return d
}

func (d *Domain) RemoveSubdomain(name string) *Domain {
	delete(d.subdomains, prepSubdomainName(name))
	return d
}

func (sd *Subdomain) SetHeader(name, value string) *Subdomain {
	sd.headers.Set(name, value)
	return sd
}

func (sd *Subdomain) SetHeaders(headers [][2]string) *Subdomain {
	for _, header := range headers {
		sd.SetHeader(header[0], header[1])
	}
	return sd
}

func (sd *Subdomain) RemoveHeader(name string) *Subdomain {
	sd.headers.Del(name)
	return sd
}

func (sd *Subdomain) Header() http.Header {
	return sd.headers
}

func (sd *Subdomain) Enable() *Subdomain {
	sd.offline = false
	return sd
}

func (sd *Subdomain) Disable() *Subdomain {
	sd.offline = true
	return sd
}

func (srv *Server) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	srv.errTemplate = t
	return nil
}

func (d *Domain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	d.errTemplate = t
	return nil
}

func (sd *Subdomain) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	sd.errTemplate = t
	return nil
}