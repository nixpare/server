package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"time"
)

type fileRangeType []struct{ filePath string; start int; end int }

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
}

// NewXFile creates a new XFile from the file with the given path. It's expected
// to find in the file other file paths (relative to the same file or absolutes),
// one for each row
func NewXFile(filePath string) (*XFile, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fileDirPath := path.Dir(filePath)

	info, _ := f.Stat()
	modTime := info.ModTime()

	x := &XFile{
		b:    &bytes.Buffer{},
		ranges: make(fileRangeType, 0),
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		info, err := os.Stat(fileDirPath + "/" + sc.Text())
		if err != nil {
			return nil, fmt.Errorf("error finding XFile part \"%s\" from \"%s\": %w", sc.Text(), filePath, err)
		}

		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		size := int(info.Size()) + 1
		x.ranges = append(x.ranges, struct{ filePath string; start int; end int }{
			filePath: fileDirPath + "/" + sc.Text(),
			start: x.size,
			end: x.size + size,
		})
		x.size += size
	}
	
	return x, nil
}

// Read is used to implement the io.Reader interface
func (x *XFile) Read(p []byte) (n int, err error) {
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
		x.b.Reset()
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
		x.b.Reset()
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
}
