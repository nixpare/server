package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/nixpare/logger/v2"
)

var (
	TimeFormat = "2006-01-02 15:04:05.00" // TimeFormat defines which timestamp to use with the logs. It can be modified.
)

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyRegistered = errors.New("already registered")
)

func (router *Router) plainPrintf(level logger.LogLevel, message string, extra string, format string, a ...any) {
	router.Logger.AddLog(level, message, extra, false)
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

// remoteAddress + code + method + requestURI + written + duration + subdomain.domain + proto (+ error)
const (
	http_info_format    = "%s%-15s%s - %s%d %s%-4s %-50s%s - %s%10.3f MB (%6d ms)%s \u279C %s%s %s(%s)%s"
	http_warning_format = "%s%-15s%s - %s%d %s%-4s %-50s%s - %s%10.3f MB (%6d ms)%s \u279C %s%s %s(%s)%s \u279C %s%s%s"
	http_error_format   = "%s%-15s%s - %s%d %s%-4s %-50s%s - %s%10.3f MB (%6d ms)%s \u279C %s%s %s(%s)%s \u279C %s%s%s"
	http_panic_format   = "%s%-15s%s - %s%d %s%-4s %-50s%s - %s%10.3f MB (%6d ms)%s \u279C %s%s %s(%s)%s \u279C %spanic: %s%s"
)

// logHTTPInfo logs http request with an exit code < 400
func (h *Handler) logHTTPInfo(m metrics) {
	h.Logger.Printf(logger.LOG_LEVEL_INFO, http_info_format,
		logger.BRIGHT_BLUE_COLOR, h.remoteAddr, logger.DEFAULT_COLOR,
		logger.BRIGHT_GREEN_COLOR, m.Code,
		logger.DARK_GREEN_COLOR, h.r.Method,
		h.r.RequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_CYAN_COLOR, h.logHost(),
		logger.BRIGHT_BLACK_COLOR, h.r.Proto, logger.DEFAULT_COLOR,
	)
}

// logHTTPWarning logs http request with an exit code >= 400 and < 500
func (h *Handler) logHTTPWarning(m metrics) {
	if h.logErrMessage == "" {
		h.logErrMessage = "Unknown error"
	}

	h.Logger.Printf(logger.LOG_LEVEL_WARNING, http_warning_format,
		logger.BRIGHT_BLUE_COLOR, h.remoteAddr, logger.DEFAULT_COLOR,
		logger.DARK_YELLOW_COLOR, m.Code,
		logger.DARK_GREEN_COLOR, h.r.Method,
		h.r.RequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_CYAN_COLOR, h.logHost(),
		logger.BRIGHT_BLACK_COLOR, h.r.Proto, logger.DEFAULT_COLOR,
		logger.DARK_YELLOW_COLOR, h.logErrMessage, logger.DEFAULT_COLOR,
	)
}

// logHTTPError logs http request with an exit code >= 500
func (h *Handler) logHTTPError(m metrics) {
	if h.logErrMessage == "" {
		h.logErrMessage = "Unknown error"
	}

	h.Logger.Printf(logger.LOG_LEVEL_ERROR, http_error_format,
		logger.BRIGHT_BLUE_COLOR, h.remoteAddr, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, m.Code,
		logger.DARK_GREEN_COLOR, h.r.Method,
		h.r.RequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_CYAN_COLOR, h.logHost(),
		logger.BRIGHT_BLACK_COLOR, h.r.Proto, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, h.logErrMessage, logger.DEFAULT_COLOR,
	)
}

func (h *Handler) logHTTPPanic(m metrics) {
	if h.logErrMessage == "" {
		h.logErrMessage = "Unknown error"
	}

	h.Logger.Printf(logger.LOG_LEVEL_FATAL, http_panic_format,
		logger.BRIGHT_BLUE_COLOR, h.remoteAddr, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, m.Code,
		logger.DARK_GREEN_COLOR, h.r.Method,
		h.r.RequestURI, logger.DEFAULT_COLOR,
		logger.BRIGHT_BLACK_COLOR, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), logger.DEFAULT_COLOR,
		logger.DARK_CYAN_COLOR, h.logHost(),
		logger.BRIGHT_BLACK_COLOR, h.r.Proto, logger.DEFAULT_COLOR,
		logger.DARK_RED_COLOR, h.logErrMessage, logger.DEFAULT_COLOR,
	)
}

func (h *Handler) logHost() string {
	if !h.redirected {
		return h.r.Host
	}

	return fmt.Sprintf("%s (%s%s)", h.r.Host, h.subdomain.name, h.domain.name)
}
