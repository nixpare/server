package server

import (
	"bytes"
	"encoding/json"
	"errors"
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
		"\n     /\\ /\\ /\\                                              /\\ /\\ /\\"+
			"\n     <> <> <> - ["+t.Format(TimeFormat)+"] - ROUTER ONLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                              \\/ \\/ \\/\n\n",
	)
}

func (router *Router) writeLogClosure(t time.Time) {
	router.plainPrintf(LOG_LEVEL_INFO, "Router Online", "",
		"\n     /\\ /\\ /\\                                               /\\ /\\ /\\"+
			"\n     <> <> <> - ["+t.Format(TimeFormat)+"] - ROUTER OFFLINE - <> <> <>"+
			"\n     \\/ \\/ \\/                                               \\/ \\/ \\/\n\n",
	)
}

// ClearLogs clears the log output file by closing, truncating and reopening it. After reopening
// it, the router will write back the welcome message with the original router start timestamp and
// an info about when the logs are cleared
func (router *Router) ClearLogs() error {
	if router.logFile == os.Stdout {
		return errors.New("can't clear logs written to stdout")
	}

	oldFile := router.logFile
	if err := oldFile.Close(); err != nil {
		return err
	}

	router.logFile, _ = os.OpenFile(oldFile.Name(), os.O_TRUNC|os.O_CREATE|os.O_WRONLY|os.O_SYNC, 0777)

	router.writeLogStart(router.startTime)
	router.plainPrintf(LOG_LEVEL_INFO, "Logs cleared", "",
		"     -- -- --   Logs cleared at [%s]   -- -- --\n\n", time.Now().Format("02/Jan/2006:15:04:05"),
	)
	return nil
}

const (
	default_terminal = "\x1b[0m"
	black_terminal = "\x1b[30m"
	dark_red_terminal = "\x1b[31m"
	dark_green_terminal = "\x1b[32m"
	dark_yellow_terminal = "\x1b[33m"
	dark_blue_terminal = "\x1b[34m"
	dark_magenta_terminal = "\x1b[35m"
	dark_cyan_terminal = "\x1b[36m"
	dark_white_terminal = "\x1b[37m"
	bright_black_terminal = "\x1b[90m"
	bright_red_terminal = "\x1b[31m"
	bright_green_terminal = "\x1b[32m"
	bright_yellow_terminal = "\x1b[33m"
	bright_blue_terminal = "\x1b[34m"
	bright_magenta_terminal = "\x1b[35m"
	bright_cyan_terminal = "\x1b[36m"
	white_terminal = "\x1b[37m"
)

var all_terminal_colors = [...]string{ default_terminal, black_terminal, dark_red_terminal, dark_green_terminal, dark_yellow_terminal,
								dark_blue_terminal, dark_magenta_terminal, dark_cyan_terminal, dark_white_terminal, bright_black_terminal,
								bright_red_terminal, bright_green_terminal, bright_yellow_terminal, bright_blue_terminal,
								bright_magenta_terminal, bright_cyan_terminal, white_terminal }

// remoteAddress + Secure/Unsecure (Lock) + Code + Method + requestURI + Written + Duration + Website Name + Domain Name + HostAddr (+ LogError)
const (
	http_info_format    = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s"
	http_warning_format = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C %s%s%s"
	http_error_format   = "%s%-15s%s - %s %s%d %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C %s%s%s"
	http_panic_format   = "%s%-15s%s - %s %s%-3s %-4s %-50s%s - %s%10.3f MB - (%6d ms)%s \u279C %s%s (%s)%s via %s%s%s \u279C panic: %s%s%s"
)

func (route *Route) getLock() string {
	lock := "\U0001F513" + dark_red_terminal + "U" + default_terminal
	if route.Secure {
		lock = "\U0001F512"  + bright_green_terminal + "S" + default_terminal
	}
	return lock
}

// logHTTPInfo logs http request with an exit code < 400
func (route *Route) logHTTPInfo(m metrics) {
	route.Logf(LOG_LEVEL_INFO, http_info_format,
		bright_blue_terminal, route.RemoteAddress, default_terminal,
		route.getLock(),
		bright_green_terminal, m.Code,
		route.R.Method,
		route.logRequestURI, default_terminal,
		bright_black_terminal, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), default_terminal,
		bright_green_terminal, route.Website.Name,
		route.Domain.Name, default_terminal,
		bright_blue_terminal, route.Host, default_terminal,
	)
}

// logHTTPWarning logs http request with an exit code >= 400 and < 500
func (route *Route) logHTTPWarning(m metrics) {
	route.Logf(LOG_LEVEL_WARNING, http_warning_format,
		bright_blue_terminal, route.RemoteAddress, default_terminal,
		route.getLock(),
		dark_yellow_terminal, m.Code,
		route.R.Method,
		route.logRequestURI, default_terminal,
		bright_black_terminal, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), default_terminal,
		dark_yellow_terminal, route.Website.Name,
		route.Domain.Name, default_terminal,
		bright_blue_terminal, route.Host, default_terminal,
		dark_yellow_terminal, route.logErrMessage, default_terminal,
	)
}

// logHTTPError logs http request with an exit code >= 500
func (route *Route) logHTTPError(m metrics) {
	route.Logf(LOG_LEVEL_FATAL, http_error_format,
		bright_blue_terminal, route.RemoteAddress, default_terminal,
		route.getLock(),
		dark_red_terminal, m.Code,
		route.R.Method,
		route.logRequestURI, default_terminal,
		bright_black_terminal, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), default_terminal,
		dark_red_terminal, route.Website.Name,
		route.Domain.Name, default_terminal,
		bright_blue_terminal, route.Host, default_terminal,
		dark_red_terminal, route.logErrMessage, default_terminal,
	)
}

func (route *Route) logHTTPPanic(m metrics) {
	code := " - "
	if m.Code != 0 {
		code = fmt.Sprint(m.Code)
	}

	route.Logf(LOG_LEVEL_FATAL, http_panic_format,
		bright_blue_terminal, route.RemoteAddress, default_terminal,
		route.getLock(),
		dark_red_terminal, code,
		route.R.Method,
		route.logRequestURI, default_terminal,
		bright_black_terminal, float64(m.Written)/1000000.,
		m.Duration.Milliseconds(), default_terminal,
		dark_red_terminal, route.Website.Name,
		route.Domain.Name, default_terminal,
		bright_blue_terminal, route.Host, default_terminal,
		dark_red_terminal, route.logErrMessage, default_terminal,
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
		data := struct {
			Code    int
			Message string
		}{
			Code:    route.W.code,
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
	LOG_LEVEL_BLANK = iota
	LOG_LEVEL_INFO
	LOG_LEVEL_DEBUG
	LOG_LEVEL_WARNING
	LOG_LEVEL_ERROR
	LOG_LEVEL_FATAL
)

func (l LogLevel) String() string {
	switch l {
	case LOG_LEVEL_BLANK:
		return ""
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
	id         string
	level      LogLevel  // Level is the Log severity (INFO - DEBUG - WARNING - ERROR - FATAL)
	date       time.Time // Date is the timestamp of the log creation
	message    string    // Message is the main message that should summarize the event
	messageRaw string
	extra      string    // Extra should hold any extra information provided for deeper understanding of the event
	extraRaw   string
}

func (l Log) Level() LogLevel {
	return l.level
}

func (l Log) Date() time.Time {
	return l.date
}

func (l Log) Message() string {
	return l.message
}

func (l Log) MessageRaw() string {
	return l.messageRaw
}

func (l Log) Extra() string {
	return l.extra
}

func (l Log) ExtraRaw() string {
	return l.extraRaw
}

// JSON returns the Log l in a json-encoded string in form of a
// slice of bytes
func (l Log) JSON() []byte {
	jsonL := struct {
		ID      string    `json:"id"`
		Level   string    `json:"level"`
		Date    time.Time `json:"date"`
		Message string    `json:"message"`
		Extra   string    `json:"extra"`
	}{
		l.id,
		strings.TrimSpace(l.level.String()), l.date,
		l.message, l.extra,
	}

	b, _ := json.Marshal(jsonL)
	return b
}

func (l Log) String() string {
	return fmt.Sprintf(
		"[%v] - %v: %s",
		l.date.Format(TimeFormat),
		l.level, l.message,
	)
}

func (l Log) Colored() string {
	var color string
	switch l.level {
	case LOG_LEVEL_INFO:
		color = bright_cyan_terminal
	case LOG_LEVEL_DEBUG:
		color = dark_magenta_terminal
	case LOG_LEVEL_WARNING:
		color = dark_yellow_terminal
	case LOG_LEVEL_ERROR:
		color = dark_red_terminal
	case LOG_LEVEL_FATAL:
		color = bright_red_terminal
	}

	return fmt.Sprintf(
		"%s[%v]%s - %s%v%s: %s",
		bright_black_terminal, l.date.Format(TimeFormat), default_terminal,
		color, l.level, default_terminal,
		l.messageRaw,
	)
}

// Full is like String(), but appends all the extra information
// associated with the log instance
func (l Log) Full() string {
	if l.extra == "" {
		return l.String()
	}

	return fmt.Sprintf(
		"[%v] - %v: %s -> %s",
		l.date.Format(TimeFormat),
		l.level, l.message,
		l.extra,
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
	Write(p []byte) (n int, err error)
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

	log := Log{
		id: fmt.Sprintf(
			"%02d%02d%02d%02d%02d%02d%03d",
			t.Year()%100, t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(), rand.Intn(1000),
		),
		level: level, date: t,
		message: removeTerminalColors(message), messageRaw: message,
		extra: removeTerminalColors(extra), extraRaw: extra,
	}

	router.logs = append(router.logs, log)
	return log
}

func removeTerminalColors(s string) string {
	for _, x := range all_terminal_colors {
		s = strings.ReplaceAll(s, x, "")
	}
	return s
}

func (router *Router) toTerminal() bool {
	stat, _ := router.logFile.Stat()
    return (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}

// Log creates a Log with the given severity and message; any data after message will be used
// to populate the extra field of the Log automatically using the built-in function
// fmt.Sprint(extra...)
func (router *Router) Log(level LogLevel, message string, extra ...any) {
	l := router.newLog(level, message, fmt.Sprint(extra...))

	if router.logFile != nil {
		if router.toTerminal() {
			if l.extra != "" {
				fmt.Fprintf(router.logFile, "%v\n%s\n", l.Colored(), IndentString(l.extra, 4))
			} else {
				fmt.Fprintln(router.logFile, l.Colored())
			}
		} else {
			if l.extra != "" {
				fmt.Fprintf(router.logFile, "%v\n%s\n", l, IndentString(l.extra, 4))
			} else {
				fmt.Fprintln(router.logFile, l)
			}
		}
	}
}

// Logf creates a Log with the given severity; the rest of the arguments is used as
// the built-in function fmt.Sprintf(format, a...), however if the resulting string
// contains a line feed, everything after that will be used to populate the extra field
// of the Log
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

func (router *Router) Write(p []byte) (n int, err error) {
	message := string(p)
	router.Logf(LOG_LEVEL_BLANK, message)
	return len(message), nil
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
// of the Log
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

func (srv *Server) Write(p []byte) (n int, err error) {
	return srv.Router.Write(p)
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
// of the Log
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

func (route *Route) Write(p []byte) (n int, err error) {
	return route.Srv.Write(p)
}
