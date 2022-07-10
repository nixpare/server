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
	route.prepareRemoteAddress()
	route.prepareHost()

	err := route.prepareRequestURI()
	if err != nil {
		route.Err = ErrBadURL
		return
	}

	route.prepareLogRequestURI()

	if !route.Srv.Online && !route.isInternalConnection() {
		route.Err = ErrServerOffline
		return
	}

	route.Err = route.parseDomainName()
}

func (route *Route) prepareRemoteAddress() {
	route.RemoteAddress = strings.Replace(route.RemoteAddress, "[::1]", "localhost", 1)
	route.RemoteAddress = strings.Replace(route.RemoteAddress, "127.0.0.1", "localhost", 1)

	if !strings.Contains(route.RemoteAddress, "localhost") {
		route.RemoteAddress = strings.Split(route.RemoteAddress, ":")[0]
	}
}

func (route *Route) prepareHost() {
	route.Host = strings.Replace(route.Host, "[::1]", "localhost", 1)
	route.Host = strings.Replace(route.Host, "127.0.0.1", "localhost", 1)
}

func (route *Route) prepareRequestURI() (err error) {
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

func (route *Route) prepareLogRequestURI() {
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

func (route *Route) isInternalConnection() bool {
	if route.RemoteAddress == "192.168.50.1" {
		return false
	}

	if strings.Contains(route.RemoteAddress, "localhost") || strings.Contains(route.RemoteAddress, "192.168.50.") || strings.Contains(route.RemoteAddress, "10.10.10.") {
		return true
	}

	return false
}

func (route *Route) parseDomainName() int {
	if route.isInternalConnection() {
		if errno := route.localParseDomainName(); errno != ErrNoErr {
			return errno
		}
	} else {
		for key := range route.Srv.domains {
			if strings.HasSuffix(route.Host, key) {
				route.Domain = key
			}
		}
	
		if route.Domain == "" {
			return ErrDomainNotFound
		}
	
		route.Subdomain = strings.Split(route.Host, route.Domain)[0]
	}

	if route.Subdomain != "" && !strings.HasSuffix(route.Subdomain, ".") {
		route.Subdomain += "."
	}

	subdomain := route.Srv.domains[route.Domain].subdomains[route.Subdomain]
	if subdomain == nil {
		return ErrSubdomainNotFound
	}

	route.Website = subdomain.website
	route.serveF = subdomain.serveFunction

	return ErrNoErr
}

func (route *Route) localParseDomainName() int {
	if strings.HasSuffix(route.Host, "localhost") {
		route.Subdomain = strings.Split(route.Host, "localhost")[0]

		if route.Subdomain != "" {
			route.Domain = "localhost"
			route.Srv.offlineClients[route.RemoteAddress] = offlineClient {
				"localhost",
				route.Subdomain,
			}

			return ErrNoErr
		}
	}

	remoteAddress := strings.Split(route.RemoteAddress, ":")[0]

	savedConfig := route.Srv.offlineClients[remoteAddress]
	route.Domain = savedConfig.domain

	queryDomain, ok := route.QueryMap["domain"]
	if ok {
		route.Domain = queryDomain
		savedConfig.domain = queryDomain
		savedConfig.subdomain = ""
		route.Srv.offlineClients[remoteAddress] = savedConfig
	}
	
	if route.Domain == "" {
		route.Domain = "localhost"
	}

	var domainFound bool
	for key := range route.Srv.domains {
		if route.Domain == key {
			domainFound = true
			break
		}
	}
	if !domainFound {
		return ErrDomainNotFound
	}

	route.Subdomain = savedConfig.subdomain

	querySubdomain, ok := route.QueryMap["subdomain"]
	if ok {
		route.Subdomain = querySubdomain
		savedConfig.subdomain = querySubdomain
		route.Srv.offlineClients[remoteAddress] = savedConfig
	}

	return ErrNoErr
}
