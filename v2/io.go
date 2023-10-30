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
	FileCacheUpdateInterval time.Duration = time.Minute * 15
)

func getCachedFile(filePath string) (cf cachedFile, found bool) {
	f, err := os.Open(filePath)
	if err != nil {
		fc.mutex.Unlock()
		return
	}
	defer f.Close()
	found = true

	cf.info, _ = f.Stat()
	cf.b, _ = io.ReadAll(f)
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

		cf, found := getCachedFile(filePath)
		if !found {
			delete(fc.m, filePath)
		} else {
			fc.m[filePath] = cf
		}
	}
}

func init() {
	go func() {
		time.Sleep(FileCacheUpdateInterval)
		UpdateFileCache()
	}()
}

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
		cf, ok = getCachedFile(filePath)
		if !ok {
			route.Error(http.StatusNotFound, "Not Found")
			return false
		}

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
