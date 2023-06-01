package server

import (
	"bytes"
	"fmt"
	"time"

	"github.com/nixpare/logger"
)

var (
	TimeFormat = "2006-01-02 15:04:05.00" // TimeFormat defines which timestamp to use with the logs. It can be modified.
)

func (router *Router) plainPrintf(level logger.LogLevel, message string, extra string, format string, a ...any) {
	log := logger.NewLog(level, message, extra)
	router.Logger.AppendLog(log)
	if out := router.Logger.Out(); out != nil {
		fmt.Fprintf(out, format, a...)
	}
}

func (router *Router) writeLogStart(t time.Time) {
	router.plainPrintf(logger.LOG_LEVEL_INFO, "Router Online", "",
		"\n     /\\ /\\ /\\                                              /\\ /\\ /\\"+
			"\n     <> <> <> - ["+t.Format(TimeFormat)+"] - ROUTER ONLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                              \\/ \\/ \\/\n\n",
	)
}

func (router *Router) writeLogClosure(t time.Time) {
	router.plainPrintf(logger.LOG_LEVEL_INFO, "Router Online", "",
		"\n     /\\ /\\ /\\                                               /\\ /\\ /\\"+
			"\n     <> <> <> - ["+t.Format(TimeFormat)+"] - ROUTER OFFLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                               \\/ \\/ \\/\n\n",
	)
}

// remoteAddress + Secure/Unsecure (Lock) + Code + Method + requestURI + Written + Duration + Website Name + Domain Name + HostAddr (+ LogError)
const (
	http_info_format    = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s"
	http_warning_format = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C %s%s%s"
	http_error_format   = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C %s%s%s"
	http_panic_format   = "%s%-15s%s - %s %s%-3s %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C panic: %s%s%s"
)

func (route *Route) getLock() string {
	lock := "\U0001F513" + logger.DARK_RED_COLOR + "U" + logger.DEFAULT_COLOR
	if route.Secure {
		lock = "\U0001F512"  + logger.BRIGHT_GREEN_COLOR + "S" + logger.DEFAULT_COLOR
	}
	return lock
}

// logHTTPInfo logs http request with an exit code < 400
func (route *Route) logHTTPInfo(m metrics) {
	route.Logger.Printf(logger.LOG_LEVEL_INFO, http_info_format,
		logger.BRIGHT_BLUE_COLOR, route.RemoteAddress, logger.DEFAULT_COLOR,
		route.getLock(),
		logger.BRIGHT_GREEN_COLOR, m.Code,
		route.R.Method,
		route.logRequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.BRIGHT_GREEN_COLOR, route.Website.Name,
		route.Domain.Name, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLUE_COLOR, route.Host, logger.DEFAULT_COLOR,
	)
}

// logHTTPWarning logs http request with an exit code >= 400 and < 500
func (route *Route) logHTTPWarning(m metrics) {
	route.Logger.Printf(logger.LOG_LEVEL_WARNING, http_warning_format,
		logger.BRIGHT_BLUE_COLOR, route.RemoteAddress, logger.DEFAULT_COLOR,
		route.getLock(),
		logger.DARK_YELLOW_COLOR, m.Code,
		route.R.Method,
		route.logRequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_YELLOW_COLOR, route.Website.Name,
		route.Domain.Name, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLUE_COLOR, route.Host, logger.DEFAULT_COLOR,
		logger.DARK_YELLOW_COLOR, route.logErrMessage, logger.DEFAULT_COLOR,
	)
}

// logHTTPError logs http request with an exit code >= 500
func (route *Route) logHTTPError(m metrics) {
	route.Logger.Printf(logger.LOG_LEVEL_FATAL, http_error_format,
		logger.BRIGHT_BLUE_COLOR, route.RemoteAddress, logger.DEFAULT_COLOR,
		route.getLock(),
		logger.DARK_RED_COLOR, m.Code,
		route.R.Method,
		route.logRequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, route.Website.Name,
		route.Domain.Name, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLUE_COLOR, route.Host, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, route.logErrMessage, logger.DEFAULT_COLOR,
	)
}

func (route *Route) logHTTPPanic(m metrics) {
	code := " - "
	if m.Code != 0 {
		code = fmt.Sprint(m.Code)
	}

	route.Logger.Printf(logger.LOG_LEVEL_FATAL, http_panic_format,
		logger.BRIGHT_BLUE_COLOR, route.RemoteAddress, logger.DEFAULT_COLOR,
		route.getLock(),
		logger.DARK_RED_COLOR, code,
		route.R.Method,
		route.logRequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, route.Website.Name,
		route.Domain.Name, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLUE_COLOR, route.Host, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, route.logErrMessage, logger.DEFAULT_COLOR,
	)
}

// serveError serves the error in a predefines error template (if set) and only
// if no other information was alredy sent to the ResponseWriter. If there is no
// error template or if the connection method is different from GET or HEAD, the
// error message is sent as a plain text
func (route *Route) serveError() {
	route.W.disableErrorCapture = true

	if len(route.W.caputedError) != 0 {
		route.errMessage = string(route.W.caputedError)
	}

	if route.errMessage == "" {
		return
	}

	if route.errTemplate == nil {
		route.ServeText(route.errMessage)
		return
	}

	if route.Method == "GET" || route.Method == "HEAD" {
		data := struct {
			Code    int
			Message string
		}{
			Code:    route.W.code,
			Message: route.errMessage,
		}

		var buf bytes.Buffer
		if err := route.errTemplate.Execute(&buf, data); err != nil {
			route.Logger.Printf(logger.LOG_LEVEL_ERROR, "Error serving template file: %v", err)
			return
		}

		route.ServeData(buf.Bytes())
		return
	}

	route.ServeText(route.errMessage)
}

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
// of the Log

// Print is a shorthand for Log(LOG_LEVE_DEBUG, a...) used for debugging

// Printf is a shorthand for Logf(LOG_LEVE_DEBUG, format, a...) used for debugging

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
// of the Log

// Print is a shorthand for Log(LOG_LEVE_DEBUG, a...) used for debugging
