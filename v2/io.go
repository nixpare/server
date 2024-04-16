package server

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/nixpare/broadcaster"
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
	vf         *VirtualFile
	info       fs.FileInfo
	expiration time.Time
}

type fileCache struct {
	m     map[string]*cachedFile
	mutex *sync.RWMutex
}

var (
	fc = fileCache{
		m: make(map[string]*cachedFile),
		mutex: new(sync.RWMutex),
	}

	fileCacheTTL time.Duration = time.Minute * 15
	cacheEnabled = false
	CachedExtensions = []string{ "", "txt", "html", "css", "js", "json" }
)

func SetFileCacheTTL(ttl time.Duration) {
	fileCacheTTL = ttl
}

func EnableFileCache() {
	cacheEnabled = true
}

func DisableFileCache() {
	cacheEnabled = false
	fc.mutex.Lock()
	defer fc.mutex.Unlock()

	for key, cacheFile := range fc.m {
		cacheFile.vf.b = nil
		delete(fc.m, key)
	}
}

func getFile(filePath string) (f *os.File, info fs.FileInfo) {
	var err error
	f, err = os.Open(filePath)
	if err != nil {
		return
	}

	info, _ = f.Stat()
	return
}

func (route *Route) httpServeFileCached(filepath string) {
	if !cacheEnabled {
		route.httpServeFile(filepath)
		return
	}

	var found bool
	_, ext, _ := strings.Cut(filepath, ".")
	for _, e := range CachedExtensions {
		if e == ext {
			found = true
			break
		}
	}
	if !found {
		route.httpServeFile(filepath)
		return
	}

	fc.mutex.RLock()
	cf, ok := fc.m[filepath]
	fc.mutex.RUnlock()
	
	if !ok || cf.expiration.Before(time.Now()) {
		cf = updateCachedFile(filepath)
		if cf == nil {
			route.Error(http.StatusNotFound, "Not Found")
			return
		}
	}

	route.Logger.Debug("Serving ...")
	route.ServeCompressedContent(
		cf.info.Name(), cf.info.ModTime(),
		cf.vf.NewReader(),
		gzip.DefaultCompression,
	)
}

func updateCachedFile(filepath string) *cachedFile {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()

	f, info := getFile(filepath)
	if info == nil {
		return nil
	}

	cf, ok := fc.m[filepath]
	if ok && info.ModTime().Equal(cf.info.ModTime()) {
		// No modifications
		f.Close()
		return cf
	}

	if !ok {
		// The file does not exist in the cache
		cf = &cachedFile{
			info: info,
			expiration: time.Now().Add(fileCacheTTL),
		}
		fc.m[filepath] = cf
	}

	cf.vf = NewVirtualFile(int(info.Size()))
	go func() {
		defer f.Close()
		defer cf.vf.bc.Close()
		io.Copy(cf.vf, f)
	}()

	return cf
}

func (route *Route) httpServeFile(filePath string) {
	f, info := getFile(filePath)
	if info == nil {
		route.Error(http.StatusNotFound, "Not Found")
		return
	}
	defer f.Close()

	route.ServeCompressedContent(
		info.Name(), info.ModTime(), f,
		gzip.DefaultCompression,
	)
}

type VirtualFile struct {
	b       []byte
	len     int
	bc      *broadcaster.Broadcaster[struct{}]
}

func NewVirtualFile(size int) *VirtualFile {
	return &VirtualFile{
		b: make([]byte, size),
		bc: broadcaster.NewBroadcaster[struct{}](),
	}
}

func (vf *VirtualFile) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	if len(b) > vf.Size() - vf.Len() {
		n = vf.Size() - vf.Len()
		err = errors.New("virtual file error: exeeded file size")
	} else {
		n = len(b)
	}

	vf.len += copy(vf.b[vf.len:], b[:n])
	vf.bc.Send(struct{}{})

	return
}

func (vf *VirtualFile) Len() int {
	return vf.len
}

func (vf *VirtualFile) Size() int {
	return cap(vf.b)
}

type virtualFileReader struct {
	vf *VirtualFile
	offset int64
}

func (vf *VirtualFile) NewReader() io.ReadSeeker {
	return &virtualFileReader{ vf: vf }
}

// Read is used to implement the io.Reader interface
func (r *virtualFileReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil // Reading no data
	}

	if int(r.offset) == r.vf.Size() {
		return 0, io.EOF // Charet position already off
	}

	var ch *broadcaster.Channel[struct{}]
	for len(p) > r.vf.Len() - int(r.offset) && r.vf.Len() < r.vf.Size() {
		if ch == nil {
			ch = r.vf.bc.Register(20)
			defer ch.Unregister()
		}

		_, ok := <- ch.Ch()
		if !ok {
			break
		}
	}

	n = copy(p, r.vf.b[r.offset:])
	r.offset += int64(n)
	if int(r.offset) == r.vf.Size() {
		err = io.EOF
	}

	return
}

// Seek is used to implement the io.Seeker interface
func (r *virtualFileReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.offset = offset
	case io.SeekCurrent:
		r.offset = int64(r.offset) + offset
	case io.SeekEnd:
		r.offset = int64(r.vf.Size()) + offset
	default:
		return 0, errors.New("virtual file seek: invalid whence")
	}

	if r.offset < 0 {
		return 0, errors.New("virtual file seek: negative position")
	}
	return r.offset, nil
}
