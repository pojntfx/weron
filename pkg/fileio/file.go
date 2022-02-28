package fileio

import (
	"io"
	"os"
	"sync"
)

type File struct {
	path string

	lock sync.Mutex

	fd *os.File
}

func NewFile(path string) *File {
	return &File{
		path: path,
	}
}

func (f *File) Open() error {
	f.lock.Lock()
	defer f.lock.Unlock()

	fd, err := os.Open(f.path)
	if err != nil {
		return err
	}

	f.fd = fd

	return nil
}

func (f *File) Read(p []byte) (n int, err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.fd == nil {
		return -1, io.ErrClosedPipe
	}

	return f.fd.Read(p)
}

func (f *File) Write(p []byte) (n int, err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.fd == nil {
		return -1, io.ErrClosedPipe
	}

	return f.fd.Write(p)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.fd == nil {
		return -1, io.ErrClosedPipe
	}

	return f.fd.Seek(offset, whence)
}

func (f *File) Close() error {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.fd == nil {
		return io.ErrClosedPipe
	}

	if err := f.fd.Close(); err != nil {
		return err
	}

	f.fd = nil

	return nil
}
