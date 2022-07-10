package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/felixge/httpsnoop"
)

func (srv *Server) WriteLogStart(t time.Time) {
	srv.FileLog.Write([]byte("\n     /\\ /\\ /\\                                            /\\ /\\ /\\"))
	srv.FileLog.Write([]byte("\n     <> <> <> - [" + t.Format("02/Jan/2006:15:04:05") + "] - SERVER ONLINE - <> <> <>"))
	srv.FileLog.Write([]byte("\n     \\/ \\/ \\/                                            \\/ \\/ \\/\n\n"))
}

func (srv *Server) WriteLogClosure(t time.Time) {
	srv.FileLog.Write([]byte("\n     /\\ /\\ /\\                                             /\\ /\\ /\\"))
	srv.FileLog.Write([]byte("\n     <> <> <> - [" + t.Format("02/Jan/2006:15:04:05") + "] - SERVER OFFLINE - <> <> <>"))
	srv.FileLog.Write([]byte("\n     \\/ \\/ \\/                                             \\/ \\/ \\/\n\n"))
}

func (srv *Server) ClearLog() {
	srv.FileLog.Close()
	srv.FileLog, _ = os.OpenFile(srv.FileLogPath, os.O_WRONLY | os.O_SYNC | os.O_TRUNC | os.O_CREATE, 0777)

	log.SetOutput(srv.FileLog)
	os.Stdout = srv.FileLog
	os.Stderr = srv.FileLog

	srv.WriteLogStart(srv.StartTimestamp)
	srv.FileLog.Write([]byte(fmt.Sprintf("     -- -- --   Logs cleared at [%s]   -- -- --\n\n", time.Now().Format("02/Jan/2006:15:04:05"))))
}

func (route *Route) logInfo(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Srv.FileLog.Write([]byte(fmt.Sprintf(
		"   Info: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s via %s\n",
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
	)))
}

func (route *Route) logWarning(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	if route.LogMessage == "" {
		route.Srv.FileLog.Write([]byte(fmt.Sprintf(
			"Warning: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s via %s\n",
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
		)))
	} else {
		route.Srv.FileLog.Write([]byte(fmt.Sprintf(
			"Warning: %-16s - [%s] - %-4s %-65s %s %d  -  %-23s \u279C %s\n",
			route.RemoteAddress,
			time.Now().Format("02/Jan/2006:15:04:05"),
			r.Method,
			route.logRequestURI,
			lock,
			metrics.Code,
			route.Host,
			route.LogMessage,
		)))
	}
}

func (route *Route) logError(r *http.Request, metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Srv.FileLog.Write([]byte(fmt.Sprintf(
		"  Error: %-16s - [%s] - %-4s %-65s %s %d  -  %-23s \u279C %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		r.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		route.Host,
		route.LogMessage,
	)))
}
