package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/nixpare/logger/v3"
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
			"\n     <> <> <> - [%s] - ROUTER ONLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                              \\/ \\/ \\/\n\n",
		t.Format(TimeFormat),
	)
}

func (router *Router) writeLogClosure(t time.Time) {
	router.plainPrintf(logger.LOG_LEVEL_INFO, "Router Offline", "",
		"\n     /\\ /\\ /\\                                               /\\ /\\ /\\"+
			"\n     <> <> <> - [%s] - ROUTER OFFLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                               \\/ \\/ \\/\n\n",
		t.Format(TimeFormat),
	)
}
