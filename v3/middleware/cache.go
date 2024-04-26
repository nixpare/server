package middleware

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	DefaultExtensions = [...]string{ "", "txt", "html", "css", "js", "json" }
)

type cachedFile struct {
	vf         *VirtualFile
	info       fs.FileInfo
	expiration time.Time
}

type Cache struct {
	m        map[string]*cachedFile
    ttl      time.Duration
    exts     []string
	mutex    *sync.RWMutex
    disabled bool
}

func NewCache(ttl time.Duration, extensions []string) *Cache {
    return &Cache{
		m:     make(map[string]*cachedFile),
        ttl:   ttl,
        exts:  extensions,
		mutex: new(sync.RWMutex),
	}
}

func (c *Cache) SetFileCacheTTL(ttl time.Duration) {
	c.ttl = ttl
}

func (c *Cache) EnableFileCache() {
    c.mutex.Lock()
    defer c.mutex.Unlock()

    if !c.disabled {
        return
    }

	c.disabled = false
}

func (c *Cache) DisableFileCache() {
	c.mutex.Lock()
    defer c.mutex.Unlock()

    if c.disabled {
        return
    }
    c.disabled = true

	for key, cacheFile := range c.m {
		cacheFile.vf.b = nil
		delete(c.m, key)
	}
}

func (c *Cache) ServeFile(w http.ResponseWriter, r *http.Request, filepath string) {
	if c.disabled {
		serveFileNoCache(w, r, filepath)
		return
	}

	var found bool
	_, ext, _ := strings.Cut(filepath, ".")
	for _, e := range c.exts {
		if e == ext {
			found = true
			break
		}
	}
	if !found {
		serveFileNoCache(w, r, filepath)
		return
	}

	c.mutex.RLock()
	cf, ok := c.m[filepath]
	c.mutex.RUnlock()
	
	if !ok || cf.expiration.Before(time.Now()) {
		cf = c.updateCachedFile(filepath)
		if cf == nil {
			w.WriteHeader(http.StatusNotFound)
            w.Write([]byte("Not Found"))
			return
		}
	}

	http.ServeContent(
        w, r,
        cf.info.Name(), cf.info.ModTime(),
        cf.vf.NewReader(),
    )
}

type cache_ctx_key_t string

const cache_ctx_key cache_ctx_key_t = "github.com/nixpare/server/v3/middlewares/cache.Cache"

func GetCache(r *http.Request) *Cache {
	a := r.Context().Value(cache_ctx_key)
	if a == nil {
		return nil
	}

	cm, ok := a.(*Cache)
	if ok {
		return nil
	}
	return cm
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

func (c *Cache) updateCachedFile(filepath string) *cachedFile {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	f, info := getFile(filepath)
	if info == nil {
		return nil
	}

	cf, ok := c.m[filepath]
	if ok && info.ModTime().Equal(cf.info.ModTime()) {
		// No modifications
		f.Close()
		return cf
	}

	if !ok {
		// The file does not exist in the cache
		cf = &cachedFile{
			info: info,
			expiration: time.Now().Add(c.ttl),
		}
		c.m[filepath] = cf
	}

	cf.vf = NewVirtualFile(int(info.Size()))
	go func() {
		defer f.Close()
		defer cf.vf.bc.Close()
		io.Copy(cf.vf, f)
	}()

	return cf
}

func serveFileNoCache(w http.ResponseWriter, r *http.Request, filepath string) {
	f, info := getFile(filepath)
	if info == nil {
        w.WriteHeader(http.StatusNotFound)
        w.Write([]byte("Not Found"))
		return
	}
	defer f.Close()

    http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
