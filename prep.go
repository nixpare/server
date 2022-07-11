package server

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	ErrNoErr = iota
	ErrBadURL
	ErrServerOffline
	ErrWebsiteOffline
	ErrDomainNotFound
	ErrSubdomainNotFound
)

func (route *Route) prep() {
	route.prepRemoteAddress()

	err := route.prepRequestURI()
	if err != nil {
		route.err = ErrBadURL
		return
	}

	route.prepLogRequestURI()

	route.DomainName, route.SubdomainName = prepDomainAndSubdomainNames(route.R)

	if route.IsInternalConn() {
		route.prepDomainAndSubdomainLocal()
	}

	route.prepHost()
	
	route.err = route.prepDomainAndSubdomain()
	route.Website = route.Subdomain.website
}

func (route *Route) prepRemoteAddress() {
	var err error
	route.RemoteAddress, _, err = net.SplitHostPort(route.R.RemoteAddr)
	if err != nil {
		route.RemoteAddress = route.R.RemoteAddr
	}
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

func prepDomainAndSubdomainNames(r *http.Request) (string, string) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		return r.Host, ""
	}

	if strings.HasSuffix(host, "127.0.0.1") || strings.HasSuffix(host, "::1") {
		return "localhost", ""
	}

	split := strings.Split(host, ".")
	len := len(split)
	
	switch len {
	case 1:
		return host, ""
	default:
		if _, err = strconv.Atoi(split[len-1]); err == nil {
			return host, ""
		}

		if strings.HasSuffix(host, "localhost") {
			return split[len-1], strings.Join(split[:len-1], ".") + "."
		}

		if len == 2 {
			return split[len-2] + "." + split[len-1], strings.Join(split[:len-2], ".")
		}

		return split[len-2] + "." + split[len-1], strings.Join(split[:len-2], ".") + "."
	}
}

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
	route.Domain = route.Srv.domains[route.DomainName]
	if route.Domain == nil {
		route.Domain = route.Srv.domains[""]
		if route.Domain == nil {
			return ErrDomainNotFound
		}
	}

	route.Subdomain = route.Domain.subdomains[route.SubdomainName]
	if route.Subdomain == nil {
		route.Subdomain = route.Domain.subdomains["*"]
		if route.Subdomain == nil {
			return ErrSubdomainNotFound
		}
	}

	return ErrNoErr
}

func (route *Route) prepDomainAndSubdomainLocal() {
	host := route.DomainName
	hostSD := route.SubdomainName

	savedConfig, ok := route.Srv.offlineClients[route.RemoteAddress]
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

	route.Srv.offlineClients[route.RemoteAddress] = offlineClient {
		route.DomainName, route.SubdomainName,
	}
}

func (route *Route) prepHost() {
	if route.SubdomainName != "" {
		route.Host = route.SubdomainName
	}

	route.Host += route.DomainName
}

func  (route *Route) IsInternalConn() bool {
	if strings.Contains(route.RemoteAddress, "localhost") || strings.Contains(route.RemoteAddress, "127.0.0.1") || strings.Contains(route.RemoteAddress, "::1") {
		return true
	}

	return route.Srv.isInternalConn(route.RemoteAddress)
}
