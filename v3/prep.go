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
	err_dipa
	err_domain_not_found                          // The domain pointed by the request was not registered on the server
	err_subdomain_not_found                       // The domain pointed by the request existed but not the subdomain
)

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
func (srv *HTTPServer) parseDomainAndSubdomain(r *http.Request) (string, string) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, _, err = net.SplitHostPort(r.Host + ":0")
		if err != nil {
			return r.Host, ""
		}
	}

	if srv.IsLocalhost(host) {
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

// prepDomainAndSubdomainLocal should be called only when the connection is local:
// this gives the capability of a local network user to access all the domains and
// subdomains served by the server from a local, insecure connection (for example
// via "http://localhost/?domain=mydomain.com&subdomain=mysubdomain").
// This feature is available for testing: an offline domain/subdomain can still be
// accessed from the local network
func (srv *HTTPServer) parseDomainAndSubdomainLocal(r *http.Request, domain string, subdomain string) (string, string) {
	srv.offlineClientsM.RLock()
	savedConfig, ok := srv.offlineClients[r.RemoteAddr]
	srv.offlineClientsM.RUnlock()

	query := r.URL.Query()

	if query.Has("domain") {
		if domainQuery := query.Get("domain"); domainQuery != "" {
			domain = domainQuery
		}	
	} else if ok {
		domain = savedConfig.domain
	}

	if query.Has("subdomain") {
		if subdomainQuery := query.Get("subdomain"); subdomainQuery != "" {
			subdomain = subdomainQuery
		}	
	} else if ok {
		subdomain = savedConfig.subdomain
	}

	subdomain = prepSubdomainName(subdomain)

	if domain != savedConfig.domain || subdomain != savedConfig.subdomain {
		srv.offlineClientsM.Lock()
		srv.offlineClients[r.RemoteAddr] = offlineClient{
			domain, subdomain,
		}
		srv.offlineClientsM.Unlock()
	}

	return domain, subdomain
}

// prepDomainAndSubdomain uses the previously parsed domain and subdomain
// to find the effective Domain and Subdomain structures and link them to
// the Route
func (srv *HTTPServer) getRoute(domainName string, subdomainName string) (*Route, routePrepError) {
	domain := srv.domains[domainName]
	if domain == nil {
		if net.ParseIP(domainName) != nil {
			return nil, err_dipa
		}

		domain = srv.domains["*"]
	}

	subdomain := domain.subdomains[subdomainName]
	if subdomain == nil {
		subdomain = domain.subdomains["*"]
		if route.Subdomain == nil {
			route.Subdomain = &Subdomain{ Name: "SNF", online: true }
			route.Website = &Website{ Name: "Not Found" }

			return err_subdomain_not_found
		}
	}

	route.Website = route.Subdomain.Website
	return err_no_err
}
