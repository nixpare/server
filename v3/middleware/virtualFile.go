package middleware

import (
	"errors"
	"io"

	"github.com/nixpare/broadcaster"
)

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
