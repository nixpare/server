package server

import (
	"fmt"
	"io"
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

func (route *Route) logInfo(metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	fmt.Fprintf(route.Srv.LogFile, "   Info: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		route.R.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		(float64(metrics.Written)/1000000.),
		time.Since(route.ConnectionTime).Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
	)
}

func (route *Route) logWarning(metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	fmt.Fprintf(route.Srv.LogFile, "Warning: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		route.R.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		(float64(metrics.Written)/1000000.),
		time.Since(route.ConnectionTime).Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
		route.logErrMessage,
	)
}

func (route *Route) logError(metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	fmt.Fprintf(route.Srv.LogFile, "Error: %-16s - [%s] - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s\n",
		route.RemoteAddress,
		time.Now().Format("02/Jan/2006:15:04:05"),
		route.R.Method,
		route.logRequestURI,
		lock,
		metrics.Code,
		(float64(metrics.Written)/1000000.),
		time.Since(route.ConnectionTime).Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
		route.logErrMessage,
	)
}

func (route *Route) serveError() {
	if route.W.written || route.errTemplate == nil {
		return
	}

	err := route.errTemplate.Execute(route.W, struct{ Code int; Message string }{ Code: route.W.code, Message: route.errMessage })
	if err != nil {
		fmt.Fprintf(route.Srv.LogFile, "Error serving template file: %v\n", err)
		return
	}
}

type LogLevel int

const (
	LOG_LEVEL_INFO = 4
	LOG_LEVEL_DEBUG = 3
	LOG_LEVEL_WARNING = 2
	LOG_LEVEL_ERROR = 1
	LOG_LEVEL_FATAL = 0
)

func logInfo(w io.Writer, a ...any) {
	fmt.Fprintf(w, "[%s]    INFO - %s", time.Now().Format(time.RFC3339), fmt.Sprint(a...))
}

func logDebug(w io.Writer, a ...any) {
	fmt.Fprintf(w, "[%s]   DEBUG - %s", time.Now().Format(time.RFC3339), fmt.Sprint(a...))
}

func logWarning(w io.Writer, a ...any) {
	fmt.Fprintf(w, "[%s] WARNING - %s", time.Now().Format(time.RFC3339), fmt.Sprint(a...))
}

func logError(w io.Writer, a ...any) {
	fmt.Fprintf(w, "[%s]   ERROR - %s", time.Now().Format(time.RFC3339), fmt.Sprint(a...))
}

func logFatal(w io.Writer, a ...any) {
	fmt.Fprintf(w, "[%s]   FATAL - %s", time.Now().Format(time.RFC3339), fmt.Sprint(a...))
}

func (srv *Server) Log(level LogLevel, a ...any) {
	switch level {
	case LOG_LEVEL_INFO:
		logInfo(srv.LogFile, a...)
	case LOG_LEVEL_DEBUG:
		logDebug(srv.LogFile, a...)
	case LOG_LEVEL_WARNING:
		logWarning(srv.LogFile, a...)
	case LOG_LEVEL_ERROR:
		logError(srv.LogFile, a...)
	case LOG_LEVEL_FATAL:
		logFatal(srv.LogFile, a...)
	}
}

func (srv *Server) Print(a ...any) {
	fmt.Fprint(srv.LogFile, a...)
}

func (srv *Server) Println(a ...any) {
	fmt.Fprintln(srv.LogFile, a...)
}

func (srv *Server) Printf(format string, a ...any) {
	fmt.Fprintf(srv.LogFile, format, a...)
}

func (route *Route) Log(level LogLevel, a ...any) {
	route.Srv.Log(level, a...)
}

func (route *Route) Print(a ...any) {
	route.Srv.Print(a...)
}

func (route *Route) Println(a ...any) {
	route.Srv.Println(a...)
}

func (route *Route) Printf(format string, a ...any) {
	route.Srv.Printf(format, a...)
}
