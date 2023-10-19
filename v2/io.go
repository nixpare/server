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

/* type fileRangeType []struct{ filePath string; start int; end int }

// XFile is a virtual file based on a buffer that chains real files
// as one. It implements the ReadSeekCloser interface. It's built to be
// capable of being called for reading even when not all the files
// are already loaded.
//
// Can be used multiple times and not all the data will be stored in memory
// indefinitely, but some could remain in the buffer, so if you are working
// with large files and you want to be sure to free up all the memory possible,
// you can call the Close method (you can still use it after the call)
type XFile struct {
	size    int           // The total size of all files
	modTime time.Time     // The most recent modTime among the x file and every contained file
	offset  int           // The actual offset reached by reading the buffer
	b       *bytes.Buffer // The underlying buffer. To see the actual written bytes use the Len() method
	ranges  fileRangeType // The list of files with their absolute offset start and stops
} */

// NewXFile creates a new XFile from the file with the given path. It's expected
// to find in the file other file paths (relative to the same file or absolutes),
// one for each row
func NewXFile(filePath string) (b []byte, modTime time.Time, err error) {
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

// Read is used to implement the io.Reader interface
/* func (x *XFile) Read(p []byte) (n int, err error) {
	switch {
	case len(p) == 0:         // Reading no data
		return 0, nil
	case x.offset == x.size:  // Charet position already off
		return 0, io.EOF
	case len(p) <= x.b.Len(): // There is enough data to be read
		n, err = x.b.Read(p)
		x.offset += n
		return
	default:
		if x.b.Len() != 0 {
			n, err = x.b.Read(p)
			if err != nil {
				return
			}
		}
		
		x.offset += n

		for _, part := range x.ranges {
			if x.offset < part.start || x.offset >= part.end {
				continue
			}

			var f *os.File
			f, err = os.Open(part.filePath)
			if err != nil {
				return
			}

			f.Seek(int64(x.offset - part.start), io.SeekStart)
			var data []byte
			data, err = io.ReadAll(f)
			if err != nil {
				return
			}
			data = append(data, '\n')

			if len(data) < len(p) - n {
				for i := 0; i < len(data); i++ {
					p[n+i] = data[i]
				}
				n += len(data)
				x.offset += len(data)

				continue
			}

			var i int
			for i = 0; i < len(p) - n; i++ {
				p[n+i] = data[i]
			}
			n += i
			x.offset += i

			x.b.Write(data[i:])
			return
		}

		return
	}
}

// Seek is used to implement the io.Seeker interface
func (x *XFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset = int(offset)
		return offset, nil
	case io.SeekCurrent:
		if x.offset+int(offset) < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset += int(offset)
		return offset, nil
	case io.SeekEnd:
		if (x.size-1)+int(offset) < 0 {
			return 0, fmt.Errorf("seek out of bound")
		}

		x.offset = (x.size - 1) + int(offset)
		return int64(x.offset), nil
	default:
		return 0, fmt.Errorf("invalid operation")
	}
}

// Size returns the total virtual file size
func (x *XFile) Size() int {
	return x.size
}

// ModTime returns the most recent modification time among
// the files source and all the files listed
func (x *XFile) ModTime() time.Time {
	return x.modTime
}

// Close is used to implement the io.Closer interface. It never
// returns an error and must not be called during a read operation
func (x *XFile) Close() error {
	x.b.Reset()
	x.offset = 0
	return nil
} */

var fileCache = make(map[string][]byte)

/* type cacheFile struct {
	size    int64
	modTime time.Time
	b       []byte
} */

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
