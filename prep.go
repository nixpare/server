package server

import (
	"net/url"
	"strings"
)

const (
	ErrNoErr = iota
	ErrBadURL
	ErrServerOffline
	ErrDomainNotFound
	ErrSubdomainNotFound
)

func (route *Route) prep() {
	route.prepRemoteAddress()
	route.prepHost()

	err := route.prepRequestURI()
	if err != nil {
		route.Err = ErrBadURL
		return
	}

	route.prepLogRequestURI()

	if !route.Srv.Online && !route.Srv.isInternalConn(route.RemoteAddress) {
		route.Err = ErrServerOffline
		return
	}

	route.Err = route.prepDomainAndSubdomain()
}

func (route *Route) prepRemoteAddress() {
	route.RemoteAddress = strings.Replace(route.RemoteAddress, "[::1]", "localhost", 1)
	route.RemoteAddress = strings.Replace(route.RemoteAddress, "127.0.0.1", "localhost", 1)

	if !strings.Contains(route.RemoteAddress, "localhost") {
		route.RemoteAddress = strings.Split(route.RemoteAddress, ":")[0]
	}
}

func (route *Route) prepHost() {
	route.Host = strings.Replace(route.Host, "[::1]", "localhost", 1)
	route.Host = strings.Replace(route.Host, "127.0.0.1", "localhost", 1)
}

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

/* func prepDomainName(r *http.Request) string {
	if strings.HasSuffix(r.Host, "localhost") {

	}
} */

func prepSubdomainName(name string) string {
	if name != "" && name != "*" && !strings.HasSuffix(name, ".") {
		name += "."
	}

	if name == "www." {
		name = ""
	}

	return name
}

func (route *Route) prepDomainAndSubdomain() int {
	if route.Srv.isInternalConn(route.RemoteAddress) {
		if errno := route.localPrepDomainAndSubdomain(); errno != ErrNoErr {
			return errno
		}
	} else {
		for key := range route.Srv.domains {
			if strings.HasSuffix(route.Host, key) {
				route.DomainName = key
			}
		}
	
		if route.DomainName == "" {
			return ErrDomainNotFound
		}
	
		route.SubdomainName = strings.Split(route.Host, route.DomainName)[0]
	}

	route.SubdomainName = prepSubdomainName(route.SubdomainName)

	domain := route.Srv.domains[route.DomainName]
	subdomain := domain.subdomains[route.SubdomainName]
	if subdomain == nil {
		subdomain = domain.subdomains["*"]
		if subdomain == nil {
			return ErrSubdomainNotFound
		}
	}

	route.Domain = domain
	route.Subdomain = subdomain

	route.Website = subdomain.website
	route.serveF = subdomain.serveFunction

	return ErrNoErr
}

func (route *Route) localPrepDomainAndSubdomain() int {
	if strings.HasSuffix(route.Host, "localhost") {
		route.SubdomainName = strings.Split(route.Host, "localhost")[0]

		if route.SubdomainName != "" {
			route.DomainName = "localhost"
			route.Srv.offlineClients[route.RemoteAddress] = offlineClient {
				"localhost",
				route.SubdomainName,
			}

			return ErrNoErr
		}
	}

	remoteAddress := strings.Split(route.RemoteAddress, ":")[0]

	savedConfig := route.Srv.offlineClients[remoteAddress]
	route.DomainName = savedConfig.domain

	queryDomain, ok := route.QueryMap["domain"]
	if ok {
		route.DomainName = queryDomain
	}
	
	if route.DomainName == "" {
		route.DomainName = "localhost"
	}

	var domainFound bool
	for key := range route.Srv.domains {
		if route.DomainName == key {
			domainFound = true
			break
		}
	}
	if !domainFound {
		return ErrDomainNotFound
	}

	route.SubdomainName = savedConfig.subdomain

	querySubdomain, ok := route.QueryMap["subdomain"]
	if ok {
		route.SubdomainName = querySubdomain
		savedConfig.subdomain = querySubdomain
		
	}

	route.Srv.offlineClients[remoteAddress] = offlineClient {
		route.DomainName, route.SubdomainName,
	}

	return ErrNoErr
}
