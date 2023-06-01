package server

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type routePrepError int

const (
	err_no_err              routePrepError = iota // No error was found when preparing the Route
	err_bad_url                                   // The request URL was not parsable or contained unsafe characters
	err_server_offline                            // The destination server for the request was set to be offline
	err_website_offline                           // The destination website for the request was set to be offline
	err_domain_not_found                          // The domain pointed by the request was not registered on the server
	err_subdomain_not_found                       // The domain pointed by the request existed but not the subdomain
)

// prep contains all the logic that prepares all the fields of
// Route before being handed over to the connection handler
// function
func (route *Route) prep() {
	route.prepRemoteAddress()

	err := route.prepRequestURI()
	if err != nil {
		route.err = err_bad_url
		route.logRequestURI = route.R.RequestURI
		return
	}

	route.prepLogRequestURI()

	route.DomainName, route.SubdomainName = prepDomainAndSubdomainNames(route.R)
	if route.IsInternalConn() {
		prepDomainAndSubdomainLocal(route)
	}

	route.err = route.prepDomainAndSubdomain()
}

// prepRemoteAddress provides the IP address of the connection client
// without the port
func (route *Route) prepRemoteAddress() {
	var err error
	route.RemoteAddress, _, err = net.SplitHostPort(route.R.RemoteAddr)
	if err != nil {
		route.RemoteAddress = route.R.RemoteAddr
	}
}

// prepRequestURI parses the requestURI incoming; in particular:
//   - it sanitizes the path of the url
//   - it sanitizes the query part of the url
//   - creates the query map looking both for keys with or without a value
func (route *Route) prepRequestURI() (err error) {
	splitPath := strings.Split(route.RequestURI, "?")
	route.RequestURI, err = url.PathUnescape(splitPath[0])
	if err != nil {
		return
	}

	var query string
	route.QueryMap = make(map[string]string)
	if len(splitPath) > 1 && splitPath[1] != "" {
		query, err = url.QueryUnescape(splitPath[1])
		if err != nil {
			return
		}

		requestQueries := strings.Split(query, "&")
		for _, x := range requestQueries {
			if strings.Contains(x, "=") {
				if strings.HasPrefix(x, "=") {
					continue
				}

				queryParsed := strings.Split(x, "=")
				route.QueryMap[queryParsed[0]] = queryParsed[1]
			} else {
				route.QueryMap[x] = ""
			}
		}
	}

	return
}

// prepLogRequestURI preformats a string used for logging containing
// information about the request uri and the queries inside
func (route *Route) prepLogRequestURI() {
	route.logRequestURI = "\"" + route.RequestURI + "\""

	if len(route.QueryMap) != 0 {
		s := " ["
		i := 0

		for key, value := range route.QueryMap {
			if i != 0 {
				s += " "
			}

			s += "<" + key + ">"

			if value != "" {
				s += ":<" + value + ">"
			}

			i++
		}

		s += "]"
		route.logRequestURI += s
	}
}

// prepDomainAndSubdomainNames parses the incoming request and separates
// the domain part from the subdomain part, just from a "string" standpoint
func prepDomainAndSubdomainNames(r *http.Request) (string, string) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, _, err = net.SplitHostPort(r.Host + ":0")
		if err != nil {
			return r.Host, ""
		}
	}

	if strings.HasSuffix(host, "127.0.0.1") || strings.HasSuffix(host, "::1") {
		return "localhost", ""
	}

	split := strings.Split(host, ".")
	length := len(split)

	switch length {
	case 1:
		return host, ""
	default:
		if _, err = strconv.Atoi(split[length-1]); err == nil {
			return host, ""
		}

		if strings.HasSuffix(host, "localhost") {
			return split[length-1], strings.Join(split[:length-1], ".") + "."
		}

		if length == 2 {
			return split[length-2] + "." + split[length-1], strings.Join(split[:length-2], ".")
		}

		return split[length-2] + "." + split[length-1], strings.Join(split[:length-2], ".") + "."
	}
}

// prepSubdomainName sanitizes the subdomain name
func prepSubdomainName(name string) string {
	if name != "" && name != "*" && !strings.HasSuffix(name, ".") {
		name += "."
	}

	return name
}

// prepDomainAndSubdomain uses the previously parsed domain and subdomain
// to find the effective Domain and Subdomain structures and link them to
// the Route
func (route *Route) prepDomainAndSubdomain() routePrepError {
	route.Domain = route.Srv.domains[route.DomainName]
	if route.Domain == nil {
		route.Domain = route.Srv.domains[""]
		if route.Domain == nil {
			route.Domain = new(Domain)
			route.Subdomain = &Subdomain{Name: ""}
			route.Website = &Website{Name: "Not Found"}

			if net.ParseIP(route.DomainName) == nil {
				route.Domain.Name = "Domain NF"
			} else {
				route.Domain.Name = "DIPA"
			}

			return err_domain_not_found
		}
	}

	route.Subdomain = route.Domain.subdomains[route.SubdomainName]
	if route.Subdomain == nil {
		route.Subdomain = route.Domain.subdomains["*"]
		if route.Subdomain == nil {
			route.Subdomain = &Subdomain{Name: "Subdomain NF"}
			route.Website = &Website{Name: "Not Found"}

			return err_subdomain_not_found
		}
	}

	route.Website = route.Subdomain.website
	return err_no_err
}

// prepDomainAndSubdomainLocal should be called only when the connection is local:
// this gives the capability of a local network user to access all the domains and
// subdomains served by the server from a local, insecure connection (for example
// via "http://localhost/?domain=mydomain.com&subdomain=mysubdomain").
// This feature is available for testing: an offline domain/subdomain can still be
// accessed from the local network
func prepDomainAndSubdomainLocal(route *Route) {
	host := route.DomainName
	hostSD := route.SubdomainName

	route.Router.offlineClientsM.RLock()
	savedConfig, ok := route.Router.offlineClients[route.RemoteAddress]
	route.Router.offlineClientsM.RUnlock()

	if ok {
		route.DomainName = savedConfig.domain
		route.SubdomainName = savedConfig.subdomain
	}

	queryDomain, ok := route.QueryMap["domain"]
	if ok {
		route.DomainName = queryDomain
	}

	if route.DomainName == "" {
		route.DomainName = host
	}

	querySubdomain, ok := route.QueryMap["subdomain"]
	if ok {
		route.SubdomainName = querySubdomain
	}

	if route.SubdomainName == "" {
		route.SubdomainName = hostSD
	}

	route.SubdomainName = prepSubdomainName(route.SubdomainName)

	if route.DomainName != savedConfig.domain || route.SubdomainName != savedConfig.subdomain {
		route.Router.offlineClientsM.Lock()
		route.Router.offlineClients[route.RemoteAddress] = offlineClient{
			route.DomainName, route.SubdomainName,
		}
		route.Router.offlineClientsM.Unlock()
	}
}
