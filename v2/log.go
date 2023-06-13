package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/nixpare/logger"
)

var (
	TimeFormat = "2006-01-02 15:04:05.00" // TimeFormat defines which timestamp to use with the logs. It can be modified.
)

var (
	ErrNotFound = errors.New("not found")
	ErrAlreadyRegistered = errors.New("already registered")
)

func (router *Router) plainPrintf(level logger.LogLevel, message string, extra string, format string, a ...any) {
	router.Logger.AddLog(level, message, extra, false)
	if out := router.Logger.Out; out != nil {
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

// Error is used to manually report an HTTP error to send to the
// client.
//
// It sets the http status code (so it should not be set
// before) and if the connection is done via a GET request, it will
// try to serve the html error template with the status code and
// error message in it, otherwise if the error template does not exist
// or the request is done via another method (like POST), the error
// message will be sent as a plain text.
//
// The last optional list of elements can be used just for logging or
// debugging: the elements will be saved in the logs
func (route *Route) Error(statusCode int, message any, a ...any) {
	route.W.WriteHeader(statusCode)
	
	route.errMessage = fmt.Sprint(message)
	if message == "" {
		route.errMessage = "Undefined error"
	}

	if len(a) > 0 {
		first := true
		for _, x := range a {
			if first {
				first = false
			} else {
				route.logErrMessage += " "
			}

			route.logErrMessage += fmt.Sprint(x)
		}
	} else {
		route.logErrMessage = route.errMessage
	}
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
	if route.logErrMessage == "" {
		route.logErrMessage = "Unknown error"
	}

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
	if route.logErrMessage == "" {
		route.logErrMessage = "Unknown error"
	}

	route.Logger.Printf(logger.LOG_LEVEL_ERROR, http_error_format,
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
	if route.logErrMessage == "" {
		route.logErrMessage = "Unknown error"
	}

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
