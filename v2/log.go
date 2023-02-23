package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

const (
	timeFormat = "2006-01-02 15:04:05.00"
)

func WriteLogStart(t time.Time) string {
	return "\n     /\\ /\\ /\\                                              /\\ /\\ /\\" +
		   "\n     <> <> <> - [" + t.Format(timeFormat) + "] - SERVER ONLINE - <> <> <>" +
		   "\n     \\/ \\/ \\/                                              \\/ \\/ \\/\n\n"
}

func WriteLogClosure(t time.Time) string {
	return "\n     /\\ /\\ /\\                                               /\\ /\\ /\\" +
		   "\n     <> <> <> - [" + t.Format(timeFormat) + "] - SERVER OFFLINE - <> <> <>" +
		   "\n     \\/ \\/ \\/                                               \\/ \\/ \\/\n\n"
}

func (router *Router) ClearLogs() {
	oldFile := router.logFile
	oldFile.Close()

	router.logFile, _ = os.OpenFile(oldFile.Name(), os.O_TRUNC | os.O_CREATE | os.O_WRONLY | os.O_SYNC, 0777)

	router.plainPrintf(WriteLogStart(router.startTime))
	router.plainPrintf("     -- -- --   Logs cleared at [%s]   -- -- --\n\n", time.Now().Format("02/Jan/2006:15:04:05"))
}

const (
	httpInfoFormat    = "%-16s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s"
	httpWarningFormat = "%-16s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s"
	httpErrorFormat   = "%-16s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s"
)

func (route *Route) logHTTPInfo(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Log(LOG_LEVEL_INFO, fmt.Sprintf(httpInfoFormat,
		route.RemoteAddress,
		route.R.Method,
		route.logRequestURI,
		lock,
		m.Code,
		(float64(m.Written)/1000000.),
		m.Duration.Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
	))
}

func (route *Route) logHTTPWarning(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Log(LOG_LEVEL_WARNING, fmt.Sprintf(httpWarningFormat,
		route.RemoteAddress,
		route.R.Method,
		route.logRequestURI,
		lock,
		m.Code,
		(float64(m.Written)/1000000.),
		m.Duration.Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
		route.logErrMessage,
	))
}

func (route *Route) logHTTPError(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Log(LOG_LEVEL_ERROR, fmt.Sprintf(httpErrorFormat,
		route.RemoteAddress,
		route.R.Method,
		route.logRequestURI,
		lock,
		m.Code,
		(float64(m.Written)/1000000.),
		m.Duration.Milliseconds(),
		route.Website.Name,
		route.Domain.Name,
		route.Host,
		route.logErrMessage,
	))
}

func (route *Route) serveError() {
	if route.W.hasWrote || route.errTemplate == nil {
		return
	}

	if route.Method == "GET" || route.Method == "HEAD" {
		data := struct{
			Code int
			Message string
		}{
			Code: route.W.code,
			Message: route.errMessage,
		}

		var buf bytes.Buffer
		if err := route.errTemplate.Execute(&buf, data); err != nil {
			route.Log(LOG_LEVEL_ERROR, "Error serving template file", err)
			return
		}

		route.ServeData(buf.Bytes())
		return
	}

	route.ServeText(route.errMessage)
}

type panicError struct {
	err 	 error
	panicErr error
	stack 	 string
}

// LogLevel defines the severity of a Log. See the constants
type LogLevel int

const (
	LOG_LEVEL_INFO = iota
	LOG_LEVEL_DEBUG
	LOG_LEVEL_WARNING
	LOG_LEVEL_ERROR
	LOG_LEVEL_FATAL
)

func (l LogLevel) String() string {
	switch l {
	case LOG_LEVEL_INFO:
		return "   Info"
	case LOG_LEVEL_DEBUG:
		return "  Debug"
	case LOG_LEVEL_WARNING:
		return "Warning"
	case LOG_LEVEL_ERROR:
		return "  Error"
	case LOG_LEVEL_FATAL:
		return "  Fatal"
	default:
		return "  ???  "
	}
}

// Log is the structure that can be will store any log reported
// with Logger. It keeps the error severity level (see the constants)
// the date it was created and the message associated with it (probably
// an error). It also has the optional field "extra" that can be used to
// store additional information
type Log struct {
	id 		string
	Level   LogLevel
	Date    time.Time
	Message string
	Extra   string
}

// JSON returns the Log l in a json-encoded string in form of a
// slice of bytes
func (l Log) JSON() []byte {
	jsonL := struct {
		ID 		string 	  `json:"id"`
		Level   string    `json:"level"`
		Date    time.Time `json:"date"`
		Message string    `json:"message"`
		Extra   string    `json:"extra"`
	}{
		l.id,
		strings.TrimSpace(l.Level.String()), l.Date,
		l.Message, l.Extra,
	}

	b, _ := json.Marshal(jsonL)
	return b
}

func (l Log) String() string {
	return fmt.Sprintf(
		"[%v] - %v: %s",
		l.Date.Format(timeFormat),
		l.Level, l.Message,
	)
}

func (l Log) Full() string {
	if l.Extra == "" {
		return l.String()
	}

	return fmt.Sprintf(
		"[%v] - %v: %s -> %s",
		l.Date.Format(timeFormat),
		l.Level, l.Message,
		l.Extra,
	)
}

// Logger is used by the Router and can be used by the user to
// create logs that are both written to the chosen io.Writer (if any)
// and saved locally in memory, so that they can be retreived
// programmatically and used (for example to make a view in a website)
type Logger interface {
	Log(level LogLevel, message string, extra ...any)
	Logs() []Log
	JSON() []byte
}

// Logs returns the list of logs stored
func (router *Router) Logs() []Log {
	logs := make([]Log, 0, len(router.logs))
	logs = append(logs, router.logs...)

	return logs
}

// JSON returns the list of logs stored in JSON format (see Log.JSON() method)
func (router *Router) JSON() []byte {
	res := make([]byte, 0)
	res = append(res, []byte("[")...)

	first := true

	for _, log := range router.logs {
		if !first {
			res = append(res, []byte(",")...)
		} else {
			first = false
		}

		res = append(res, log.JSON()...)
	}

	res = append(res, []byte("]")...)
	return res
}

// Router
func (router *Router) Log(level LogLevel, message string, extra ...any) {
	t := time.Now()
	
	router.mLog.Lock()
	defer router.mLog.Unlock()

	log := Log {
		fmt.Sprintf(
			"%02d%02d%02d%02d%02d%02d%03d",
			t.Year() % 100, t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(), rand.Intn(1000),
		), level, t,
		message, fmt.Sprint(extra...),
	}

	if router.logFile != nil {
		if log.Extra != "" {
			fmt.Fprintf(router.logFile, "%v\n%s\n", log, IndentString(log.Extra, 1))
		} else {
			fmt.Fprintln(router.logFile, log)
		}		
	}
}

func (router *Router) Print(a ...any) {
	var v []string
	for _, el := range a {
		v = append(v, fmt.Sprint(el))
	}

	router.Log(LOG_LEVEL_DEBUG, strings.Join(v, " "))
}

func (router *Router) Printf(format string, a ...any) {
	router.Log(LOG_LEVEL_DEBUG, fmt.Sprintf(format, a...))
}

func (router *Router) plainPrintf(format string, a ...any) {
	fmt.Fprintf(router.logFile, format, a...)
}

// Server

func (srv *Server) Log(level LogLevel, message string, a ...any) {
	srv.Router.Log(level, message, a...)
}

func (srv *Server) Print(a ...any) {
	srv.Router.Print(a...)
}

func (srv *Server) Printf(format string, a ...any) {
	srv.Router.Printf(format, a...)
}

// Route

func (route *Route) Log(level LogLevel, message string, a ...any) {
	route.Srv.Log(level, message, a...)
}

func (route *Route) Print(a ...any) {
	route.Srv.Print(a...)
}

func (route *Route) Printf(format string, a ...any) {
	route.Srv.Printf(format, a...)
}
