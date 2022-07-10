package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type xFile struct {
	size int
	offset int
	b *bytes.Buffer
	c chan struct{}
}

func (route *Route) StaticServe(serveHTML bool) {
	StaticServe(route, route.w, route.r, serveHTML)
}

func (route *Route) ServeFile(path string) {
	ServeFile(route.w, route.r, path)
}

func (route *Route) ServeRootedFile(path string) {
	ServeFile(route.w, route.r, route.Website.Dir + path)
}

func (route *Route) ServePlainData(name string, data []byte) {
	ServePlainData(route.w, route.r, name, data)
}

func (route *Route) ServePlainText(name, text string) {
	ServePlainText(route.w, route.r, name, text)
}

func ServeFile(w http.ResponseWriter, r *http.Request, path string) {
	if strings.Contains(path, "..") {
		http.Error(w, "Bad request URL", http.StatusBadRequest)
		return
	}
	
	fileInfo, err := os.Stat(path)
	if err == nil {
		if fileInfo.IsDir() {
			http.Error(w, "404 page not found", http.StatusNotFound)
			return
		}
	
		http.ServeFile(w, r, path)
		return
	}

	fileInfo, err = os.Stat(path + ".html")
	if err != nil {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(path + ".html")
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), f)
}

func ServePlainData(w http.ResponseWriter, r *http.Request, name string, data []byte) {
	http.ServeContent(w, r, name, time.Now(), bytes.NewReader(data))
}

func ServePlainText(w http.ResponseWriter, r *http.Request, name, text string) {
	http.ServeContent(w, r, name, time.Now(), bytes.NewReader([]byte(text)))
}

func StaticServe(route *Route, w http.ResponseWriter, r *http.Request, serveHTML bool) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if route.RequestURI == "/" {
		ServeFile(w, r, route.Website.Dir + "/index.html")
		return
	}

	if strings.HasSuffix(route.RequestURI, ".html") && !serveHTML {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}

	if strings.Count(route.RequestURI, "/") == 1 {
		ServeFile(w, r, route.Website.Dir + route.RequestURI)
		return
	}

	for _, s := range route.Website.AllFolders {
		if strings.HasPrefix(route.RequestURI, s) {
			if strings.HasSuffix(route.RequestURI, ".css") && route.Website.EnableCSSX {
				serveCSSX(route, w, r)
				return
			}

			ServeFile(w, r, route.Website.Dir + route.RequestURI)
			return
		}
	}

	http.Error(w, "404 page not found", http.StatusNotFound)
}

func serveCSSX(route *Route, w http.ResponseWriter, r *http.Request) {
	info, err := os.Stat(route.Website.Dir + route.RequestURI + "x")
	if err != nil {
		http.Error(w, "Error 404 not found", http.StatusNotFound)
		return
	}

	cssx, err := os.Open(route.Website.Dir + route.RequestURI + "x")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Printf("Error opening %s file\n", route.Website.Dir + route.RequestURI + "x")
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
			data, err := ioutil.ReadFile(s)
			if err != nil {
				continue
			}

			css.Write(data)
			css.Write([]byte("\n\n"))
		}
	}()

	http.ServeContent(w, r, basePathSplit[len(basePathSplit)-1], modTime, css)
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
