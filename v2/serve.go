package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Error is used to manually report an http error to send to the
// client. It sets the http status code (so it should not be set
// before) and if the connection is done via a GET request, it will
// try to serve the html error template with the status code and
// error message in it, otherwise if the error template does not exist
// or the request is done via another method (like POST), the error
// message will be sent as a plain text.
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

// ServeFile will serve a file in the file system. If the path is not
// absolute, it will first try to complete it with the website directory
// (if set) or with the server path
func (route *Route) ServeFile(filePath string) {
	if !path.IsAbs(filePath) {
		if route.Website.Dir != "" {
			filePath = route.Website.Dir + "/" + filePath
		} else {
			filePath = route.Srv.ServerPath + "/" + filePath
		}
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
		route.Error(http.StatusInternalServerError, "Error retreiving page", fmt.Sprintf("Error opening file %s: %v", filePath + ".html", err))
		return
	}
	defer f.Close()

	http.ServeContent(route.W, route.R, fileInfo.Name(), fileInfo.ModTime(), f)
}

// ServeCustomFileWithTime will serve a pseudo-file saved in memory specifing the
// last modification time
func (route *Route) ServeCustomFileWithTime(name string, data []byte, t time.Time) {
	http.ServeContent(route.W, route.R, "", t, bytes.NewReader(data))
}

// ServeCustomFileWithTime will serve a pseudo-file saved in memory
func (route *Route) ServeCustomFile(name string, data []byte) {
	http.ServeContent(route.W, route.R, "", time.Now(), bytes.NewReader(data))
}

// ServeData serves raw bytes to the client
func (route *Route) ServeData(data []byte) {
	route.W.Write(data)
}

// ServeData serves a string (as raw bytes) to the client
func (route *Route) ServeText(text string) {
	route.ServeData([]byte(text))
}

// StaticServe tries to serve a file for every connection done via
// a GET request, following all the options provided in the Website
// configuration. This means it will not serve any file inside (also
// nested) an hidden folder, it will serve an html file only with the
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
			if strings.HasSuffix(route.RequestURI, ".css") && route.Website.EnableCSSX {
				route.serveCSSX()
				return
			}

			route.ServeFile(route.Website.Dir + route.RequestURI)
			return
		}
	}

	route.Error(http.StatusNotFound, "Not Found")
}

// TODO
type xFile struct {
	size int
	offset int
	b *bytes.Buffer
	c chan struct{}
}

// TODO 
func (route *Route) serveCSSX() {
	basePath := route.Website.Dir
	if basePath == "" {
		basePath = route.Srv.ServerPath
	}

	filePath := basePath + "/" + route.RequestURI

	info, err := os.Stat(filePath + "x")
	if err != nil {
		route.ServeFile(route.Website.Dir + route.RequestURI)
		return
	}

	filePath = filePath + "x"

	cssx, err := os.Open(filePath)
	if err != nil {
		route.Error(http.StatusInternalServerError, "Internal server error", fmt.Sprintf("Error opening file %s: %v", filePath, err))
		return
	}
	defer cssx.Close()

	pathSplit := strings.Split(basePath, "/")
	var fileDirPath string
	for _, s := range pathSplit[:len(pathSplit)-1] {
		fileDirPath += s + "/"
	}

	var size int
	modTime := info.ModTime()
	fileNames := make([]string, 0)

	sc := bufio.NewScanner(cssx)
	for sc.Scan() {
		info, err = os.Stat(fileDirPath + sc.Text())
		if err != nil {
			continue
		}

		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		size += int(info.Size()) + 2
		fileNames = append(fileNames, fileDirPath + sc.Text())
	}

	css := newXFile(size)
	go func() {
		for _, s := range fileNames {
			data, err := os.ReadFile(s)
			if err != nil {
				continue
			}

			css.Write(data)
			css.Write([]byte("\n\n"))
		}
	}()

	http.ServeContent(route.W, route.R, pathSplit[len(pathSplit)-1], modTime, css)
}


func (route *Route) SetCookie(name string, value interface{}, maxAge int) error {
	encValue, err := route.Srv.secureCookie.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(route.W, &http.Cookie {
		Name: GenerateHashString([]byte(name)),
		Value: encValue,
		Domain: route.DomainName,
		MaxAge: maxAge,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

func (route *Route) DeleteCookie(name string) {
	http.SetCookie(route.W, &http.Cookie{
		Name: GenerateHashString([]byte(name)),
		Value: "",
		Domain: route.DomainName,
		MaxAge: -1,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})
}

func (route *Route) DecodeCookie(name string, value interface{}) (bool, error) {
	if cookie, err := route.R.Cookie(GenerateHashString([]byte(name))); err == nil {
		return true, route.Srv.secureCookie.Decode(name, cookie.Value, value)
	}
	
	return false, nil
}

func (route *Route) SetCookiePerm(name string, value interface{}, maxAge int) error {
	encValue, err := route.Srv.secureCookiePerm.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(route.W, &http.Cookie{
		Name: GenerateHashString([]byte(name)),
		Value: encValue,
		Domain: route.DomainName,
		MaxAge: maxAge,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

func (route *Route) DecodeCookiePerm(name string, value interface{}) (bool, error) {
	if cookie, err := route.R.Cookie(GenerateHashString([]byte(name))); err == nil {
		return true, route.Srv.secureCookiePerm.Decode(name, cookie.Value, value)
	}
	
	return false, nil
}

func (route *Route) ReverseProxy(URL string) error {
	urlParsed, err := url.Parse(URL)
	if err != nil {
		return err
	}

	proxyServer := httputil.NewSingleHostReverseProxy(urlParsed)

	noLogFile, _ := os.Open(os.DevNull)
	proxyServer.ErrorLog = log.New(noLogFile, "", 0)

	proxyServer.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		route.Error(http.StatusBadGateway, "Bad gateway", err.Error())
	}

	proxyServer.ServeHTTP(route.W, route.R)
	return nil
}

func newXFile(len int) *xFile {
	return &xFile {
		size: len,
		b: &bytes.Buffer{},
		c: make(chan struct{}, 10),
	}
}

func (x *xFile) Write(p []byte) {
	x.b.Write(p)
	x.c <- struct{}{}
}

func (x *xFile) Read(p []byte) (n int, err error) {
	switch {
	case len(p) == 0:
		return 0, nil
	case x.offset >= x.size:
		return 0, io.EOF
	case x.b.Len() == x.size || len(p) <= x.b.Len() - x.offset:
		data := x.b.Bytes()
		var i int
		for i = 0; i < len(p) && i < x.b.Len() - x.offset; i++ {
			p[i] = data[x.offset+i]
		}

		x.offset += i
		return i, nil
	default:
		for {
			<- x.c
			if len(p) <= x.b.Len() - x.offset {
				return x.Read(p)
			}
		}
	}
}

func (x *xFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset = int(offset)
		return offset, nil
	case io.SeekCurrent:
		if x.offset + int(offset) < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset += int(offset)
		return offset, nil
	case io.SeekEnd:
		if (x.size -1 ) + int(offset) < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset = (x.size - 1) + int(offset)
		return int64(x.offset), nil
	default:
		return 0, fmt.Errorf("invalid operation")
	}
}

func ReadJSON[T any](route *Route) (value T, err error) {
	var data []byte
	data, err = io.ReadAll(route.R.Body)
	if err != nil {
		return value, err
	}

	if err = json.Unmarshal(data, &value); err != nil {
		return value, err
	}

	return value, nil
}
