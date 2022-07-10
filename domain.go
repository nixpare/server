package server

type Domain struct {
	name string
	subdomains map[string]*Subdomain
}

type Subdomain struct {
	name string
	website *Website
	serveFunction ServeFunction
	initFunction InitFunction
	offline bool
}

func (d *Domain) RegisterSubdomain(name string, website Website, serveF ServeFunction, initF InitFunction) {
	name = prepSubdomainName(name)

	if serveF == nil {
		website.AllFolders = []string{""}
		serveF = func(route *Route) { route.StaticServe(true) }
	}

	ws := new(Website)
	*ws = website

	d.subdomains[name] = &Subdomain {
		name, ws,
		serveF, initF,
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