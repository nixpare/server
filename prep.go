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
		for key := range route.Srv.DomainsMap {
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

	route.RR = route.Srv.DomainsMap[route.Domain].SubdomainRules[route.Subdomain]
	if route.RR == nil {
		if route.Subdomain == "www." {
			route.RR = route.Srv.DomainsMap[route.Domain].SubdomainRules[""]
			if route.RR == nil {
				return ErrSubdomainNotFound
			}
		} else {
			return ErrSubdomainNotFound
		}
	}

	return ErrNoErr
}

func (route *Route) localParseDomainName() int {
	if strings.HasSuffix(route.Host, "localhost") {
		route.Subdomain = strings.Split(route.Host, "localhost")[0]

		if route.Subdomain != "" {
			route.Domain = "localhost"
			route.Srv.OfflineClients[route.RemoteAddress] = OfflineClientConf {
				Domain: "localhost",
				Subdomain: route.Subdomain,
			}

			return ErrNoErr
		}
	}

	remoteAddress := strings.Split(route.RemoteAddress, ":")[0]

	savedConfig := route.Srv.OfflineClients[remoteAddress]
	route.Domain = savedConfig.Domain

	queryDomain, ok := route.QueryMap["domain"]
	if ok {
		route.Domain = queryDomain
		savedConfig.Domain = queryDomain
		savedConfig.Subdomain = ""
		route.Srv.OfflineClients[remoteAddress] = savedConfig
	}
	
	if route.Domain == "" {
		route.Domain = "localhost"
	}

	var domainFound bool
	for key := range route.Srv.DomainsMap {
		if route.Domain == key {
			domainFound = true
			break
		}
	}
	if !domainFound {
		return ErrDomainNotFound
	}

	route.Subdomain = savedConfig.Subdomain

	querySubdomain, ok := route.QueryMap["subdomain"]
	if ok {
		route.Subdomain = querySubdomain
		savedConfig.Subdomain = querySubdomain
		route.Srv.OfflineClients[remoteAddress] = savedConfig
	}

	return ErrNoErr
}
