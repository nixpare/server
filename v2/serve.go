package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

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
func (route *Route) Error(statusCode int, message string, a ...any) {
	route.W.WriteHeader(statusCode)

	route.errMessage = message
	if message == "" {
		route.errMessage = "Undefined error"
	}

	route.logErrMessage = route.errMessage
	if len(a) > 0 {
		v := make([]string, 0, len(a))
		for _, el := range a {
			v = append(v, fmt.Sprint(el))
		}
		route.logErrMessage = strings.Join(v, " ")
	}
}

// Errorf is like the method Route.Error but you can format the output
// to the Log. Like the Route.Logf, everything that is after the first
// line feed will be used to populate the extra field of the Log
func (route *Route) Errorf(statusCode int, message string, format string, a ...any) {
	route.Error(statusCode, message, fmt.Sprintf(format, a...))
}

// ServeFile will serve a file in the file system. If the path is not
// absolute, it will first try to complete it with the website directory
// (if set) or with the server path
func (route *Route) ServeFile(filePath string) {
	if !isAbs(filePath) {
		filePath = route.Website.Dir + "/" + filePath
	}

	if strings.HasPrefix(filePath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = strings.Replace(filePath, "~", home, 1)
		}
	}

	if strings.Contains(filePath, "..") {
		route.Error(http.StatusBadRequest, "Bad request URL", "URL contains ..")
		return
	}

	if value, ok := route.Website.XFiles[strings.TrimLeft(route.RequestURI, "/")]; ok {
		if !isAbs(value) {
			value = route.Website.Dir + "/" + value
		}
		route.serveXFile(value)
		return
	}

	fileInfo, err := os.Stat(filePath)
	if err == nil {
		if fileInfo.IsDir() {
			route.Error(http.StatusNotFound, "Not found", "Cannot serve directory", filePath)
			return
		}

		http.ServeFile(route.W, route.R, filePath)
		return
	}

	fileInfo, err = os.Stat(filePath + ".html")
	if err != nil {
		route.Error(http.StatusNotFound, "Not found")
		return
	}

	f, err := os.Open(filePath + ".html")
	if err != nil {
		route.Error(http.StatusInternalServerError, "Error retreiving page", fmt.Sprintf("Error opening file %s: %v", filePath+".html", err))
		return
	}
	defer f.Close()

	http.ServeContent(route.W, route.R, fileInfo.Name(), fileInfo.ModTime(), f)
}

func (route *Route) serveXFile(xFilePath string) {
	x, err := NewXFile(xFilePath)
	if err != nil {
		route.Error(
			http.StatusInternalServerError,
			"Internal server error",
			err,
		)
		return
	}

	http.ServeContent(route.W, route.R, route.RequestURI, x.ModTime(), x)
}

// ServeCustomFileWithTime will serve a pseudo-file saved in memory specifing the
// last modification time. The name of the file is important for MIME type detection
func (route *Route) ServeCustomFileWithTime(fileName string, data []byte, t time.Time) {
	http.ServeContent(route.W, route.R, fileName, t, bytes.NewReader(data))
}

// ServeCustomFile serves a pseudo-file saved in memory. The name of the file is
// important for MIME type detection
func (route *Route) ServeCustomFile(fileName string, data []byte) {
	http.ServeContent(route.W, route.R, fileName, time.Now(), bytes.NewReader(data))
}

// ServeData serves raw bytes to the client
func (route *Route) ServeData(data []byte) {
	route.W.Write(data)
}

// ServeText serves a string (as raw bytes) to the client
func (route *Route) ServeText(text string) {
	route.ServeData([]byte(text))
}

// StaticServe tries to serve a file for every connection done via
// a GET request, following all the options provided in the Website
// configuration. This means it will not serve any file inside (also
// nested) a hidden folder, it will serve an HTML file only with the
// flag argument set to true, it will serve index.html automatically
// for connection with request uri empty or equal to "/", it will serve
// every file inside the AllFolders field of the Website
func (route *Route) StaticServe(serveHTML bool) {
	if route.Method != "GET" && route.Method != "HEAD" {
		route.Error(http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	for _, s := range route.Website.HiddenFolders {
		if s == "" || strings.HasPrefix(route.RequestURI, s) {
			route.Error(http.StatusNotFound, "Not Found")
			return
		}
	}

	if route.RequestURI == "/" && serveHTML {
		route.ServeFile(route.Website.Dir + "/index.html")
		return
	}

	if strings.HasSuffix(route.RequestURI, ".html") && !serveHTML {
		route.Error(http.StatusNotFound, "Not Found")
		return
	}

	for _, s := range route.Website.AllFolders {
		if s == "" || strings.HasPrefix(route.RequestURI, s) {
			route.ServeFile(route.Website.Dir + route.RequestURI)
			return
		}
	}

	route.Error(http.StatusNotFound, "Not Found")
}

// SetCookie creates a new cookie with the given name and value, maxAge can be used
// to sex the expiration date:
//   - maxAge = 0 means no expiration specified
//   - maxAge > 0 sets the expiration date from the current date adding the given time in seconds
//     (- maxAge < 0 will remove the cookie instantly, like route.DeleteCookie)
//
// The cookie value is encoded and encrypted using a pair of keys created randomly at server creation,
// so if the same cookie is later decoded between server restart, it can't be decoded. To have such a
// behaviour see SetCookiePerm.
//
// The encoding of the value is managed by the package encoding/gob. If you are just encoding and decoding
// plain structs and each field type is a primary type or a struct (with the same rules), nothing should be
// done, but if you are dealing with interfaces, you must first register every concrete structure or type
// implementing that interface before encoding or decoding
func (route *Route) SetCookie(name string, value any, maxAge int) error {
	encValue, err := route.Srv.secureCookie.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(route.W, &http.Cookie{
		Name:     GenerateHashString([]byte(name)),
		Value:    encValue,
		Domain:   route.DomainName,
		MaxAge:   maxAge,
		Secure:   route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

// DeleteCookie instantly removes a cookie with the given name before set with route.SetCookie
// or route.SetCookiePerm
func (route *Route) DeleteCookie(name string) {
	http.SetCookie(route.W, &http.Cookie{
		Name:     GenerateHashString([]byte(name)),
		Value:    "",
		Domain:   route.DomainName,
		MaxAge:   -1,
		Secure:   route.Secure,
		HttpOnly: route.Secure,
	})
}

// DecodeCookie decodes a previously set cookie with the given name
// using the method route.SetCookie.
//
// If the cookie was not found, it will return false and the relative error
// (probably an http.ErrNoCookie), otherwise it will return true and, possibly,
// the decode error. It happends when:
//  + the server was restarted, so the keys used for decoding are different
//  + you provided the wrong value type
//  + the cookie was not set by the server
//
// The argument value must be a pointer, otherwise the value will not
// be returned. A workaround might be using the type parametric
// function server.DecodeCookie
func (route *Route) DecodeCookie(name string, value any) (found bool, err error) {
	cookie, err := route.R.Cookie(GenerateHashString([]byte(name)))
	if err != nil {
		return
	}

	return true, route.Srv.secureCookie.Decode(name, cookie.Value, value)
}

// DecodeCookie decodes a previously set cookie with the given name
// using the method route.SetCookie and returns the saved value.
// This is a duplicate function of the method route.DecodeCookie as
// a type parameter function.
//
// More informations provided in that method
func DecodeCookie[T any](route *Route, name string) (value T, found bool, err error) {
	found, err = route.DecodeCookie(name, &value)
	return
}

// SetCookiePerm creates a new cookie with the given name and value, maxAge can be used
// to sex the expiration date:
//   - maxAge = 0 means no expiration specified
//   - maxAge > 0 sets the expiration date from the current date adding the given time in seconds
//     (- maxAge < 0 will remove the cookie instantly, like route.DeleteCookie)
//
// The cookie value is encoded and encrypted using a pair of keys at package level that MUST be set at
// program startup. This differs for the method route.SetCookie to ensure that even after server restart
// these cookies can still be decoded.
func (route *Route) SetCookiePerm(name string, value any, maxAge int) error {
	encValue, err := route.Srv.secureCookiePerm.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(route.W, &http.Cookie{
		Name:     GenerateHashString([]byte(name)),
		Value:    encValue,
		Domain:   route.DomainName,
		MaxAge:   maxAge,
		Secure:   route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

// DecodeCookiePerm decodes a previously set cookie with the given name
// using the method route.SetCookiePerm.
//
// If the cookie was not found, it will return false and the relative error
// (probably an http.ErrNoCookie), otherwise it will return true and, possibly,
// the decode error. It happends when:
//  + you provided the wrong value type
//  + the cookie was not set by the server
//
// The argument value must be a pointer, otherwise the value will not
// be returned. A workaround might be using the type parametric
// function server.DecodeCookiePerm
func (route *Route) DecodeCookiePerm(name string, value any) (found bool, err error) {
	cookie, err := route.R.Cookie(GenerateHashString([]byte(name)))
	if err != nil {
		return
	}

	return true, route.Srv.secureCookiePerm.Decode(name, cookie.Value, value)
}

// DecodeCookiePerm decodes a previously set cookie with the given name
// using the method route.SetCookiePerm and returns the saved value.
// This is a duplicate function of the method route.DecodeCookiePerm as
// a type parameter function
//
// More informations provided in that method
func DecodeCookiePerm[T any](route *Route, name string) (value T, found bool, err error) {
	found, err = route.DecodeCookiePerm(name, &value)
	return
}

// ReverseProxy runs a reverse proxy to the provided url. Returns an error is the
// url could not be parsed or if an error has occurred during the connection
func (route *Route) ReverseProxy(URL string) error {
	urlParsed, err := url.Parse(URL)
	if err != nil {
		return err
	}

	proxyServer := httputil.NewSingleHostReverseProxy(urlParsed)
	proxyServer.ErrorLog = log.New(route.Router, fmt.Sprintf("PROXY [%s]", URL), 0)

	errChan := make(chan error)
	defer close(errChan)

	proxyServer.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		errChan <- err
	}

	go func() {
		proxyServer.ServeHTTP(route.W, route.R)
		errChan <- nil
	}()

	return <-errChan
}

// RespBody returns the response body bytes
func (route *Route) RespBody() ([]byte, error) {
	return io.ReadAll(route.R.Body)
}

// RespBodyString returns the response body as a string
func (route *Route) RespBodyString() (string, error) {
	data, err := route.RespBody()
	return string(data), err
}

// ReadJSON unmarshals the response body and returns the value
func ReadJSON[T any](route *Route) (value T, err error) {
	data, err := route.RespBody()
	if err != nil {
		return
	}

	if err = json.Unmarshal(data, &value); err != nil {
		return value, err
	}

	return value, nil
}

// IsInternalConn tells wheather the incoming connection should be treated
// as a local connection. The user can add a filter that can extend this
// selection to match their needs
func (route *Route) IsInternalConn() bool {
	if strings.Contains(route.RemoteAddress, "localhost") || strings.Contains(route.RemoteAddress, "127.0.0.1") || strings.Contains(route.RemoteAddress, "::1") {
		return true
	}

	return route.Router.IsInternalConn(route.RemoteAddress)
}
