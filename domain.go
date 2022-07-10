package server

type Domain struct {
	Name string
	subdomains map[string]*Subdomain
}

type Subdomain struct {
	Name string
	website *Website
	serveFunction ServeFunction
	initFunction InitFunction
	offline bool
}

type SubdomainConfig struct {
	Name string
	Website Website
	ServeF ServeFunction
	InitF InitFunction
}

func (d *Domain) RegisterSubdomain(c SubdomainConfig) {
	c.Name = prepSubdomainName(c.Name)

	if c.ServeF == nil {
		c.Website.AllFolders = []string{""}
		c.ServeF = func(route *Route) { route.StaticServe(true) }
	}

	ws := new(Website)
	*ws = c.Website

	d.subdomains[c.Name] = &Subdomain {
		c.Name, ws,
		c.ServeF, c.InitF,
		false,
	}
}

func (d *Domain) EnableSubdomain(name string) {
	name = prepSubdomainName(name)
	
	sd := d.subdomains[name]
	if (sd != nil) {
		sd.Enable()
	}
}

func (d *Domain) DisableSubdomain(name string) {
	name = prepSubdomainName(name)
	
	sd := d.subdomains[name]
	if (sd != nil) {
		sd.Disable()
	}
}

func (d *Domain) RemoveSubdomain(name string) {
	delete(d.subdomains, prepSubdomainName(name))
}

func (sd *Subdomain) Enable() {
	sd.offline = false
}

func (sd *Subdomain) Disable() {
	sd.offline = true
}