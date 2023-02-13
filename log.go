package server

import (
	"fmt"
	"io"
	"time"

	"github.com/felixge/httpsnoop"
)

const LogFormat string = "02/Jan/2006:15:04:05"

func WriteLogStart(t time.Time) string {
	return "\n     /\\ /\\ /\\                                            /\\ /\\ /\\" +
		   "\n     <> <> <> - [" + t.Format(LogFormat) + "] - SERVER ONLINE - <> <> <>" +
		   "\n     \\/ \\/ \\/                                            \\/ \\/ \\/\n\n"
}

func WriteLogClosure(t time.Time) string {
	return "\n     /\\ /\\ /\\                                             /\\ /\\ /\\" +
		   "\n     <> <> <> - [" + t.Format(LogFormat) + "] - SERVER OFFLINE - <> <> <>" +
		   "\n     \\/ \\/ \\/                                             \\/ \\/ \\/\n\n"
}

func (router *Router) ClearLogs() {
	router.logFile.Truncate(0)

	router.Print(router.startTime)
	router.Print(fmt.Sprintf("     -- -- --   Logs cleared at [%s]   -- -- --\n\n", time.Now().Format("02/Jan/2006:15:04:05")))
}

func (srv *Server) LogFile() io.Writer {
	if srv.Router == nil {
		return srv.logFile
	}

	return srv.Router.logFile
}

func (route *Route) logInfo(metrics httpsnoop.Metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Srv.Logf(LOG_LEVEL_INFO, "%-16s - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s\n",
		route.RemoteAddress,
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

	route.Srv.Logf(LOG_LEVEL_WARNING, "%-16s - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s\n",
		route.RemoteAddress,
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

	route.Srv.Logf(LOG_LEVEL_ERROR, "%-16s - %-4s %-65s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s\n",
		route.RemoteAddress,
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
		route.Srv.Logf(LOG_LEVEL_ERROR, "Error serving template file: %v\n", err)
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
		logInfo(srv.LogFile(), a...)
	case LOG_LEVEL_DEBUG:
		logDebug(srv.LogFile(), a...)
	case LOG_LEVEL_WARNING:
		logWarning(srv.LogFile(), a...)
	case LOG_LEVEL_ERROR:
		logError(srv.LogFile(), a...)
	case LOG_LEVEL_FATAL:
		logFatal(srv.LogFile(), a...)
	}
}

func (srv *Server) Logf(level LogLevel, format string, a ...any) {
	switch level {
	case LOG_LEVEL_INFO:
		logInfo(srv.LogFile(), fmt.Sprintf(format, a...))
	case LOG_LEVEL_DEBUG:
		logDebug(srv.LogFile(), fmt.Sprintf(format, a...))
	case LOG_LEVEL_WARNING:
		logWarning(srv.LogFile(), fmt.Sprintf(format, a...))
	case LOG_LEVEL_ERROR:
		logError(srv.LogFile(), fmt.Sprintf(format, a...))
	case LOG_LEVEL_FATAL:
		logFatal(srv.LogFile(), fmt.Sprintf(format, a...))
	}
}

func (srv *Server) Logln(level LogLevel, format string, a ...any) {
	srv.Log(level, append(a, "\n"))
}

func (srv *Server) Print(a ...any) {
	fmt.Fprint(srv.LogFile(), a...)
}

func (srv *Server) Println(a ...any) {
	fmt.Fprintln(srv.LogFile(), a...)
}

func (srv *Server) Printf(format string, a ...any) {
	fmt.Fprintf(srv.LogFile(), format, a...)
}

func (router *Router) Log(level LogLevel, a ...any) {
	switch level {
	case LOG_LEVEL_INFO:
		logInfo(router.logFile, a...)
	case LOG_LEVEL_DEBUG:
		logDebug(router.logFile, a...)
	case LOG_LEVEL_WARNING:
		logWarning(router.logFile, a...)
	case LOG_LEVEL_ERROR:
		logError(router.logFile, a...)
	case LOG_LEVEL_FATAL:
		logFatal(router.logFile, a...)
	}
}

func (router *Router) Logf(level LogLevel, format string, a ...any) {
	switch level {
	case LOG_LEVEL_INFO:
		logInfo(router.logFile, fmt.Sprintf(format, a...))
	case LOG_LEVEL_DEBUG:
		logDebug(router.logFile, fmt.Sprintf(format, a...))
	case LOG_LEVEL_WARNING:
		logWarning(router.logFile, fmt.Sprintf(format, a...))
	case LOG_LEVEL_ERROR:
		logError(router.logFile, fmt.Sprintf(format, a...))
	case LOG_LEVEL_FATAL:
		logFatal(router.logFile, fmt.Sprintf(format, a...))
	}
}

func (router *Router) Logln(level LogLevel, format string, a ...any) {
	router.Log(level, append(a, "\n"))
}

func (router *Router) Print(a ...any) {
	fmt.Fprint(router.logFile, a...)
}

func (router *Router) Println(a ...any) {
	fmt.Fprintln(router.logFile, a...)
}

func (router *Router) Printf(format string, a ...any) {
	fmt.Fprintf(router.logFile, format, a...)
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
