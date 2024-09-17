package lib

import (
	"os"
	"path"
	"syscall"
)

func CreateFile(elem ...string) (ret *os.File, err error) {
	fullPath := path.Join(elem...)
	dir := path.Dir(fullPath)

	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	return os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0666) // alloc
}

type MmapFile struct {
	*os.File
	Data []byte
	size int
}

func (mf *MmapFile) Init(size int, elem ...string) (err error) {
	fullPath := path.Join(elem...)
	dir := path.Dir(fullPath)
	_, err = os.Stat(dir)

	if err != nil {
		if !os.IsNotExist(err) {
			return
		}

		err = os.MkdirAll(dir, 0700)
	}

	if err != nil {
		return
	}

	mf.size = size

	f, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return
	}

	err = syscall.Ftruncate(int(f.Fd()), int64(mf.size))
	if err != nil {
		return
	}

	data, err := syscall.Mmap(
		int(f.Fd()), 0, mf.size,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED,
	)

	if err != nil {
		return
	}

	mf.File = f
	mf.Data = data

	return
}

func (mf *MmapFile) Close() error {
	defer mf.File.Close()
	return syscall.Munmap(mf.Data)
}
