package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"
)

// XFile is a virtual file based on a buffer that chains real files
// as one. It implements the ReadSeekCloser interface. It's built to be
// capable of being called for reading even when not all the files
// are already loaded.
// Every content of all the files are stored in memory so when you
// no longer need to access there file you should close it to free
// up some memory
type XFile struct {
	size    int           // The total size of all files
	modTime time.Time     // The most recent modTime among the x file and every contained file
	offset  int           // The actual offset reached by reading the buffer
	b       *bytes.Buffer // The underlying buffer. To see the actual written bytes use the Len() method
	// This channel is used to communicate every time a file has been written and to wait for this event
	// when trying to read bytes that are not already accessible
	c      chan struct{}
	closed bool // Tells whether the file was closed
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

	var size int
	info, _ := f.Stat()
	modTime := info.ModTime()
	fileNames := make([]string, 0)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		info, err := os.Stat(fileDirPath + sc.Text())
		if err != nil {
			return nil, fmt.Errorf("error finding XFile part \"%s\" from \"%s\": %w", sc.Text(), filePath, err)
		}

		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
		}

		size += int(info.Size()) + 1
		fileNames = append(fileNames, fileDirPath+sc.Text())
	}

	x := &XFile{
		size: size,
		b:    &bytes.Buffer{},
		c:    make(chan struct{}, 10),
	}
	go func() {
		for _, s := range fileNames {
			filePart, err := os.Open(s)
			if err != nil {
				panic(err)
			}

			_, err = io.Copy(&xFileWriter{x}, filePart)
			if err != nil {
				panic(err)
			}

			x.write([]byte("\n"))
		}
	}()

	return x, nil
}

// Read is used to implement the io.Reader interface
func (x *XFile) Read(p []byte) (n int, err error) {
	if x.closed {
		return 0, errors.New("xfile already closed")
	}

	switch {
	case len(p) == 0:
		return 0, nil
	case x.offset >= x.size:
		return 0, io.EOF
	case x.b.Len() == x.size || len(p) <= x.b.Len()-x.offset:
		data := x.b.Bytes()
		var i int
		for i = 0; i < len(p) && i < x.b.Len()-x.offset; i++ {
			p[i] = data[x.offset+i]
		}

		x.offset += i
		return i, nil
	default:
		for {
			<-x.c
			if len(p) <= x.b.Len()-x.offset {
				return x.Read(p)
			}
		}
	}
}

// Seek is used to implement the io.Seeker interface
func (x *XFile) Seek(offset int64, whence int) (int64, error) {
	if x.closed {
		return offset, errors.New("xfile already closed")
	}

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

func (x *XFile) write(p []byte) (n int, err error) {
	defer func() { x.c <- struct{}{} }()
	return x.b.Write(p)
}

// xFileWriter is used to make XFile implement the io.Writer interface
type xFileWriter struct {
	x *XFile
}

func (x xFileWriter) Write(p []byte) (n int, err error) {
	return x.x.write(p)
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
	if x.closed {
		return nil
	}

	x.closed = true
	x.b = nil
	return nil
}
