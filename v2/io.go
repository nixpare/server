package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
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
	var parts []string
	var size int64

	for sc.Scan() {
		filePartPath := fileDirPath + "/" + sc.Text()
		info, err = os.Stat(filePartPath)
		if err != nil {
			err = fmt.Errorf("error finding XFile part \"%s\" from \"%s\": %w", sc.Text(), filePath, err)
			return
		}

		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		if info.Size() == 0 {
			continue
		}

		parts = append(parts, filePartPath)
		size += info.Size()
	}

	b = make([]byte, size)
	var lastRead int
	for _, p := range parts {
		var fPart *os.File
		fPart, err = os.Open(p)
		if err != nil {
			err = fmt.Errorf("error opening XFile part \"%s\" from \"%s\": %w", p, filePath, err)
			return
		}

		var n int
		n, err = fPart.Read(b[lastRead:])
		if err != nil {
			err = fmt.Errorf("error reading XFile part \"%s\" from \"%s\": %w", p, filePath, err)
			return
		}

		lastRead += n
	}
	
	return
}

var fileCache = make(map[string][]byte)

func (route *Route) httpServeFileCached(filePath string, fileInfo fs.FileInfo) {
	cf, ok := fileCache[filePath]
	if ok {
		http.ServeContent(
			route.W, route.R,
			fileInfo.Name(), fileInfo.ModTime(),
			bytes.NewReader(cf),
		)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		route.Error(http.StatusInternalServerError, "Internal cache failed", "Internal cache failed on", filePath, err)
		return
	}

	fileCache[filePath] = content

	http.ServeContent(
		route.W, route.R,
		fileInfo.Name(), fileInfo.ModTime(),
		bytes.NewReader(content),
	)
}
