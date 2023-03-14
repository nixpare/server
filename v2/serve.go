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

// xFile
type xFile struct {
	size int
	offset int
	b *bytes.Buffer
	c chan struct{}
}

func (route *Route) Error(statusCode int, message string, a ...any) {
	route.W.WriteHeader(statusCode)

	route.errMessage = message
	if message == "" {
		route.errMessage = "Undefined error"
	}

	route.logErrMessage = route.errMessage
	if len(a) > 0 {
		var v []string
		for _, el := range a {
			v = append(v, fmt.Sprint(el))
		}
		route.logErrMessage = strings.Join(v, " ")
	}
}

func (route *Route) ServeRootedFile(filePath string) {
	route.ServeFile(route.Website.Dir + filePath)
}

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

func (route *Route) ServeCustomFileWithTime(name string, data []byte, t time.Time) {
	http.ServeContent(route.W, route.R, "", t, bytes.NewReader(data))
}

func (route *Route) ServeCustomFile(name string, data []byte) {
	http.ServeContent(route.W, route.R, "", time.Now(), bytes.NewReader(data))
}

func (route *Route) ServeData(data []byte) {
	route.W.Write(data)
}

func (route *Route) ServeText(text string) {
	route.ServeData([]byte(text))
}

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

func (route *Route) serveCSSX() {
	info, err := os.Stat(route.Website.Dir + route.RequestURI + "x")
	if err != nil {
		route.ServeFile(route.Website.Dir + route.RequestURI)
		return
	}

	cssx, err := os.Open(route.Website.Dir + route.RequestURI + "x")
	if err != nil {
		route.Error(http.StatusInternalServerError, "Internal server error", fmt.Sprintf("Error opening file %s: %v", route.Website.Dir + route.RequestURI + "x", err))
		return
	}
	defer cssx.Close()

	basePathSplit := strings.Split(route.Website.Dir + route.RequestURI, "/")
	var basePath string
	for _, s := range basePathSplit[:len(basePathSplit)-1] {
		basePath += s + "/"
	}

	var size int
	modTime := info.ModTime()
	fileNames := make([]string, 0)

	sc := bufio.NewScanner(cssx)
	for sc.Scan() {
		info, err = os.Stat(basePath + sc.Text())
		if err != nil {
			continue
		}

		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		size += int(info.Size()) + 2
		fileNames = append(fileNames, basePath + sc.Text())
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

	http.ServeContent(route.W, route.R, basePathSplit[len(basePathSplit)-1], modTime, css)
}

func (route *Route) SetCookie(name string, value interface{}, maxAge int) error {
	encValue, err := route.Srv.secureCookie.Encode(name, value)
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
