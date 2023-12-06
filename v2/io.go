package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
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

	ticker *time.Ticker
	fileCacheUpdateInterval time.Duration = time.Minute * 15
	cacheEnabled = true
)

func SetFileCacheUpdateInterval(d time.Duration) {
	fileCacheUpdateInterval = d
	ticker.Reset(fileCacheUpdateInterval)
}

func EnableFileCache() {
	cacheEnabled = true
	ticker.Reset(fileCacheUpdateInterval)
}

func DisableFileCache() {
	cacheEnabled = false
	ticker.Stop()
	for key := range fc.m {
		delete(fc.m, key)
	}
}

func getFile(filePath string) (content []byte, info fs.FileInfo, found bool) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()
	found = true

	info, _ = f.Stat()
	content, _ = io.ReadAll(f)
	return
}

func UpdateFileCache() {
	for filePath, cf := range fc.m {
		newInfo, err := os.Stat(filePath)
		if err != nil || newInfo.IsDir() {
			delete(fc.m, filePath)
		}

		if !newInfo.ModTime().After(cf.info.ModTime()) {
			continue
		}

		content, info, found := getFile(filePath)
		if !found {
			delete(fc.m, filePath)
		} else {
			fc.m[filePath] = cachedFile{
				b: content,
				info: info,
			}
		}
	}
}

func init() {
	go func() {
		ticker = time.NewTicker(fileCacheUpdateInterval)
		
		for range ticker.C {
			UpdateFileCache()
		}
	}()
}

func (route *Route) httpServeFileCached(filePath string) bool {
	if !cacheEnabled {
		return route.httpServeFile(filePath)
	}

	fc.mutex.RLock()
	cf, ok := fc.m[filePath]
	fc.mutex.RUnlock()
	if ok {
		route.ServeCompressedContent(
			cf.info.Name(), cf.info.ModTime(),
			bytes.NewReader(cf.b),
			gzip.DefaultCompression,
		)
		return true
	}

	fc.mutex.Lock()
	cf, ok = fc.m[filePath]
	if !ok {
		content, info, found := getFile(filePath)
		if !found {
			fc.mutex.Unlock()
			route.Error(http.StatusNotFound, "Not Found")
			return false
		}

		cf = cachedFile{
			b: content,
			info: info,
		}
		fc.m[filePath] = cf
	}
	fc.mutex.Unlock()

	route.ServeCompressedContent(
		cf.info.Name(), cf.info.ModTime(),
		bytes.NewReader(cf.b),
		gzip.DefaultCompression,
	)
	return true
}

func (route *Route) httpServeFile(filePath string) bool {
	content, info, found := getFile(filePath)
	if !found {
		route.Error(http.StatusNotFound, "Not Found")
		return false
	}

	route.ServeCompressedContent(
		info.Name(), info.ModTime(),
		bytes.NewReader(content),
		gzip.DefaultCompression,
	)
	return true
}
