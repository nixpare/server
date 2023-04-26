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

var (
	TimeFormat = "2006-01-02 15:04:05.00" // TimeFormat defines which timestamp to use with the logs. It can be modified.
)

func (router *Router) writeLogStart(t time.Time) {
	router.plainPrintf(LOG_LEVEL_INFO, "Router Online", "",
		"\n     /\\ /\\ /\\                                              /\\ /\\ /\\" +
		"\n     <> <> <> - [" + t.Format(TimeFormat) + "] - ROUTER ONLINE - <> <> <>" +
		"\n     \\/ \\/ \\/                                              \\/ \\/ \\/\n\n",
	)
}

func (router *Router) writeLogClosure(t time.Time) {
	router.plainPrintf(LOG_LEVEL_INFO, "Router Online", "",
		"\n     /\\ /\\ /\\                                               /\\ /\\ /\\" +
		"\n     <> <> <> - [" + t.Format(TimeFormat) + "] - ROUTER OFFLINE - <> <> <>" +
		"\n     \\/ \\/ \\/                                               \\/ \\/ \\/\n\n",
	) 
}

// ClearLogs clears the log output file by closing, truncating and reopening it. After reopening
// it, the router will write back the welcome message with the original router start timestamp and
// an info about when the logs are cleared
func (router *Router) ClearLogs() error {
	oldFile := router.logFile
	if err := oldFile.Close(); err != nil {
		router.Logf(LOG_LEVEL_ERROR, "Can't clear logs: %v", err)
		return err
	}

	router.logFile, _ = os.OpenFile(oldFile.Name(), os.O_TRUNC | os.O_CREATE | os.O_WRONLY | os.O_SYNC, 0777)

	router.writeLogStart(router.startTime)
	router.plainPrintf(LOG_LEVEL_INFO, "Logs cleared", "",
		"     -- -- --   Logs cleared at [%s]   -- -- --\n\n", time.Now().Format("02/Jan/2006:15:04:05"),
	)
	return nil
}

// remoteAddress + Method + requestURI + Secure/Unsecure + Code + Written + Duration + Website Name + Domain Name + HostAddr (+ LogError)
const (
	httpInfoFormat    = "%-15s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s"
	httpWarningFormat = "%-15s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s"
	httpErrorFormat   = "%-15s - %-4s %-50s %s %d %10.3f MB - (%6d ms) \u279C %s (%s) via %s \u279C %s"
)

// logHTTPInfo logs http request with an exit code < 400
func (route *Route) logHTTPInfo(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Logf(LOG_LEVEL_INFO, httpInfoFormat,
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
	)
}

// logHTTPWarning logs http request with an exit code >= 400 and < 500
func (route *Route) logHTTPWarning(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Log(LOG_LEVEL_WARNING, httpWarningFormat,
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
	)
}

// logHTTPError logs http request with an exit code >= 500
func (route *Route) logHTTPError(m metrics) {
	lock := "\U0000274C"
	if route.Secure {
		lock = "\U0001F512"
	}

	route.Log(LOG_LEVEL_ERROR, httpErrorFormat,
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
	)
}

// serveError serves the error in a predefines error template (if set) and only
// if no other information was alredy sent to the ResponseWriter. If there is no
// error template or if the connection method is different from GET or HEAD, the
// error message is sent as a plain text
func (route *Route) serveError() {
	if route.W.hasWrote {
		return
	}

	if route.errTemplate == nil {
		route.ServeText(route.errMessage)
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
	Level   LogLevel 		// Level is the Log severity (INFO - DEBUG - WARNING - ERROR - FATAL)
	Date    time.Time 		// Date is the timestamp of the log creation
	Message string 			// Message is the main message that should summarize the event
	Extra   string 			// Extra should hold any extra information provided for deeper understanding of the event
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
		l.Date.Format(TimeFormat),
		l.Level, l.Message,
	)
}

// Full is like String(), but appends all the extra informations
// associated with the log instance
func (l Log) Full() string {
	if l.Extra == "" {
		return l.String()
	}

	return fmt.Sprintf(
		"[%v] - %v: %s -> %s",
		l.Date.Format(TimeFormat),
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
	Logf(level LogLevel, format string, a ...any)
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

func (router *Router) newLog(level LogLevel, message string, extra string) Log {
	t := time.Now()
	
	router.logMutex.Lock()
	defer router.logMutex.Unlock()

	log := Log {
		fmt.Sprintf(
			"%02d%02d%02d%02d%02d%02d%03d",
			t.Year() % 100, t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(), rand.Intn(1000),
		), level, t,
		message, extra,
	}

	router.logs = append(router.logs, log)
	return log
}

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)
func (router *Router) Log(level LogLevel, message string, extra ...any) {
	log := router.newLog(level, message, fmt.Sprint(extra...))

	if router.logFile != nil {
		if log.Extra != "" {
			fmt.Fprintf(router.logFile, "%v\n%s\n", log, IndentString(log.Extra, 4))
		} else {
			fmt.Fprintln(router.logFile, log)
		}		
	}
}

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
func (router *Router) Logf(level LogLevel, format string, a ...any) {
	str := fmt.Sprintf(format, a...)
	message, extra, _ := strings.Cut(str, "\n")
	router.Log(level, message, extra)
}

// Print is a shorthand for Log(LOG_LEVE_DEBUG, a...) used for debugging
func (router *Router) Print(a ...any) {
	var v []string
	for _, el := range a {
		v = append(v, fmt.Sprint(el))
	}

	router.Log(LOG_LEVEL_DEBUG, strings.Join(v, " "))
}

// Printf is a shorthand for Logf(LOG_LEVE_DEBUG, format, a...) used for debugging
func (router *Router) Printf(format string, a ...any) {
	router.Log(LOG_LEVEL_DEBUG, fmt.Sprintf(format, a...))
}

func (router *Router) plainPrintf(level LogLevel, message string, extra string, format string, a ...any) {
	router.newLog(level, message, extra)
	if router.logFile != nil {
		fmt.Fprintf(router.logFile, format, a...)
	}
}

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)
func (srv *Server) Log(level LogLevel, message string, a ...any) {
	srv.Router.Log(level, message, a...)
}

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
func (srv *Server) Logf(level LogLevel, format string, a ...any) {
	srv.Router.Logf(level, format, a...)
}

// Print is a shorthand for Log(LOG_LEVE_DEBUG, a...) used for debugging
func (srv *Server) Print(a ...any) {
	srv.Router.Print(a...)
}

// Printf is a shorthand for Logf(LOG_LEVE_DEBUG, format, a...) used for debugging
func (srv *Server) Printf(format string, a ...any) {
	srv.Router.Printf(format, a...)
}

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)
func (route *Route) Log(level LogLevel, message string, a ...any) {
	route.Srv.Log(level, message, a...)
}

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
func (route *Route) Logf(level LogLevel, format string, a ...any) {
	route.Srv.Logf(level, format, a...)
}

// Print is a shorthand for Log(LOG_LEVE_DEBUG, a...) used for debugging
func (route *Route) Print(a ...any) {
	route.Srv.Print(a...)
}

// Printf is a shorthand for Logf(LOG_LEVE_DEBUG, format, a...) used for debugging
func (route *Route) Printf(format string, a ...any) {
	route.Srv.Printf(format, a...)
}
