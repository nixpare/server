package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
)

func (srv *Server) WriteLogStart(t time.Time) {
	fmt.Fprint(srv.LogFile, "\n     /\\ /\\ /\\                                            /\\ /\\ /\\")
	fmt.Fprint(srv.LogFile, "\n     <> <> <> - [" + t.Format("02/Jan/2006:15:04:05") + "] - SERVER ONLINE - <> <> <>")
	fmt.Fprint(srv.LogFile, "\n     \\/ \\/ \\/                                            \\/ \\/ \\/\n\n")
}

func (srv *Server) WriteLogClosure(t time.Time) {
	fmt.Fprint(srv.LogFile, "\n     /\\ /\\ /\\                                             /\\ /\\ /\\")
	fmt.Fprint(srv.LogFile, "\n     <> <> <> - [" + t.Format("02/Jan/2006:15:04:05") + "] - SERVER OFFLINE - <> <> <>")
	fmt.Fprint(srv.LogFile, "\n     \\/ \\/ \\/                                             \\/ \\/ \\/\n\n")
}

func (route *Route) logInfo(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	fmt.Fprintf(route.Srv.LogFile, "   Info: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s via %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		r.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		(float64(metrics.Written)/1000000.),
		time.Since(route.ConnectionTime).Milliseconds(),
		route.RR.Website.Name,
		route.Subdomain + route.Domain,
	)
}

func (route *Route) logWarning(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	if route.LogMessage == "" {
		fmt.Fprintf(route.Srv.LogFile, "Warning: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s via %s\n",
			route.RemoteAddress,
			time.Now().Format("02/Jan/2006:15:04:05"),
			r.Method,
			route.logRequestURI,
			lock,
			metrics.Code,
			(float64(metrics.Written)/1000000.),
			time.Since(route.ConnectionTime).Milliseconds(),
			route.RR.Website.Name,
			route.Subdomain + route.Domain,
		)
	} else {
		fmt.Fprintf(route.Srv.LogFile, "Warning: %-16s - [%s] - %-4s %-65s %s %d  -  %-23s \u279C %s\n",
			route.RemoteAddress,
			time.Now().Format("02/Jan/2006:15:04:05"),
			r.Method,
			route.logRequestURI,
			lock,
			metrics.Code,
			route.Host,
			route.LogMessage,
		)
	}
}

func (route *Route) logError(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	fmt.Fprintf(route.Srv.LogFile, "  Error: %-16s - [%s] - %-4s %-65s %s %d  -  %-23s \u279C %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		r.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		route.Host,
		route.LogMessage,
	)
}
