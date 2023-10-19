package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

func newXFile(filePath string) (b []byte, modTime time.Time, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	fileDirPath := path.Dir(filePath)

	info, _ := f.Stat()
	modTime = info.ModTime()

	sc := bufio.NewScanner(f)
	var parts []*os.File
	var size int64

	for sc.Scan() {
		filePartPath := fileDirPath + "/" + sc.Text()
		var part *os.File
		part, err = os.Open(filePartPath)

		if err != nil {
			err = fmt.Errorf("error finding XFile part \"%s\" from \"%s\": %w", sc.Text(), filePath, err)
			return
		}

		info, _ := part.Stat()
		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		if info.Size() == 0 {
			continue
		}

		parts = append(parts, part)
		size += info.Size()
	}

	b = make([]byte, size)
	var lastRead int
	for i, p := range parts {
		var n int
		n, err = p.Read(b[lastRead:])
		p.Close()
		if err != nil {
			err = fmt.Errorf("error reading XFile part %d from \"%s\": %w", i, filePath, err)
			return
		}

		lastRead += n
	}
	
	return
}

type cachedFile struct {
	b    []byte
	info fs.FileInfo
}

type fileCache struct {
	m     map[string]cachedFile
	mutex *sync.RWMutex
}

var (
	fc = fileCache{
		m: make(map[string]cachedFile),
		mutex: new(sync.RWMutex),
	}
)

func (route *Route) httpServeFileCached(filePath string) bool {
	fc.mutex.RLock()
	cf, ok := fc.m[filePath]
	fc.mutex.RUnlock()
	if ok {
		http.ServeContent(
			route.W, route.R,
			cf.info.Name(), cf.info.ModTime(),
			bytes.NewReader(cf.b),
		)
		return true
	}

	fc.mutex.Lock()
	cf, ok = fc.m[filePath]
	if !ok {
		f, err := os.Open(filePath)
		if err != nil {
			fc.mutex.Unlock()
			return route.Error(http.StatusNotFound, "Not found")
		}

		cf.info, _ = f.Stat()
		cf.b, _ = io.ReadAll(f)

		fc.m[filePath] = cf
	}
	fc.mutex.Unlock()

	http.ServeContent(
		route.W, route.R,
		cf.info.Name(), cf.info.ModTime(),
		bytes.NewReader(cf.b),
	)
	return true
}
