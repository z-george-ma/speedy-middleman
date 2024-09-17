package remotechannel

import (
	"container/list"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"
	"unsafe"
)

type Memfile struct {
	indexPath  string
	dataPath   string
	data       *os.File
	dataHead   uint32
	index      *Index
	state      *State
	notifyItem []chan struct{}
	l          sync.Mutex
}

func openOrCreateFile(file string, flag int, perm fs.FileMode, size int) (isNewFile bool, f *os.File, err error) {
	_, err = os.Stat(file)
	isNewFile = os.IsNotExist(err)

	if !isNewFile && err != nil {
		return
	}

	f, err = os.OpenFile(file, flag, perm)

	if err != nil {
		return
	}

	err = syscall.Ftruncate(int(f.Fd()), int64(size))
	if err != nil {
		f.Close()
		f = nil
		return
	}

	return
}

const stateFileSize = 4096

func (m *Memfile) initState(stateFile string) error {
	if err := os.MkdirAll(path.Dir(stateFile), 0700); err != nil {
		return err
	}

	shouldInitState, f, err := openOrCreateFile(stateFile, os.O_RDWR|os.O_CREATE|os.O_SYNC, 0666, stateFileSize)

	if err != nil {
		return err
	}

	data, err := syscall.Mmap(
		int(f.Fd()), 0, stateFileSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED,
	)

	if err != nil {
		f.Close()
		return err
	}

	m.state = &State{
		File: f,
		Data: data,
	}

	m.state.Init(data, shouldInitState)
	return nil
}

func (m *Memfile) createData(page uint64) (*os.File, error) {
	fileName := path.Join(m.dataPath, strconv.Itoa(int(page)))
	return os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
}

func (m *Memfile) createIndex(page uint64) (*Index, error) {
	fileName := path.Join(m.indexPath, strconv.Itoa(int(page)))
	fileSize := IndexCount * 8
	_, f, err := openOrCreateFile(fileName, os.O_RDWR|os.O_CREATE, 0666, fileSize)

	if err != nil {
		return nil, err
	}

	data, err := syscall.Mmap(
		int(f.Fd()), 0, fileSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED,
	)

	if err != nil {
		f.Close()
		return nil, err
	}

	return &Index{
		Data:   data,
		File:   f,
		Offset: (*[IndexCount]Offset)(unsafe.Pointer(&data[0])),
	}, nil
}

func (m *Memfile) Init(indexPath string, dataPath string, stateFile string) error {
	m.indexPath = indexPath
	m.dataPath = dataPath

	if err := os.MkdirAll(m.indexPath, 0700); err != nil {
		return err
	}

	if err := os.MkdirAll(m.dataPath, 0700); err != nil {
		return err
	}

	if err := m.initState(stateFile); err != nil {
		return err
	}

	var currentPage uint64 = 0
	if *m.state.Head > 0 {
		currentPage = (*m.state.Head - 1) / IndexCount
	}

	index, err := m.createIndex(currentPage)
	if err != nil {
		m.Close()
		return err
	}
	m.index = index

	data, err := m.createData(currentPage)
	if err != nil {
		m.Close()
		return err
	}

	m.data = data
	if *m.state.Head == currentPage*IndexCount {
		return nil
	}

	indexOffset := m.index.Offset[*m.state.Head-1-currentPage*IndexCount]
	m.dataHead = indexOffset.offset + indexOffset.length
	_, err = m.data.Seek(int64(m.dataHead), 0)
	return err
}

func (m *Memfile) Close() (ret error) {
	ret = syscall.Munmap(m.state.Data)

	if m.index != nil {
		if e := syscall.Munmap(m.index.Data); ret == nil && e != nil {
			ret = e
		}
		if e := m.index.File.Close(); ret == nil && e != nil {
			ret = e
		}
	}

	if m.data != nil {
		if e := m.data.Close(); ret == nil && e != nil {
			ret = e
		}
	}

	if e := m.state.File.Close(); ret == nil && e != nil {
		ret = e
	}

	return
}

const IndexCount = 4 * 1024

type Offset struct {
	offset uint32
	length uint32
}

type Index struct {
	Data   []byte
	File   *os.File
	Offset *[IndexCount]Offset
}

type Cursor struct {
	memfile     *Memfile
	dataFile    *os.File
	offset      *uint64 // id of max acked message
	head        uint64  // id of max published message
	lock        sync.Mutex
	trackerList *list.List
	trackerMap  map[uint64]*list.Element
}

func (m *Memfile) Register(sub string) *Cursor {
	// reset to latest for new subscription
	m.l.Lock()
	head := *m.state.Head
	m.l.Unlock()

	offset, head, err := m.state.GetOrAddSub(sub, head)

	if err != nil {
		return nil
	}

	return &Cursor{
		memfile:     m,
		offset:      offset,
		head:        head,
		trackerList: list.New(),
		trackerMap:  make(map[uint64]*list.Element),
	}
}

func (m *Memfile) Add(item *DeliveryItem, callback DeliverCallback) (err error) {
	if *m.state.Head%IndexCount == 1 {
		// clean up old files
		minSubHead := uint64(0)
		for _, sub := range m.state.Sub {
			if minSubHead == 0 || *sub.Head < minSubHead {
				minSubHead = *sub.Head
			}
		}

		if minSubHead > 0 {
			minPageToKeep := (minSubHead - 1) / IndexCount
			for i := *m.state.EarliestPage; i < minPageToKeep; i++ {
				os.Remove(path.Join(m.indexPath, strconv.Itoa(int(i))))
				os.Remove(path.Join(m.dataPath, strconv.Itoa(int(i))))
			}

			*m.state.EarliestPage = minPageToKeep
		}
	}

	l := uint32(len(item.Data))

	index := m.index
	data := m.data
	dataHead := m.dataHead
	closeOldPage := false
	prevIndex := m.index
	prevData := m.data

	if item.Id != 1 && item.Id%IndexCount == 1 {
		// create current page
		currentPage := (item.Id - 1) / IndexCount
		index, err = m.createIndex(currentPage)
		if err != nil {
			goto _EXIT
		}

		data, err = m.createData(currentPage)
		if err != nil {
			goto _EXIT
		}

		dataHead = 0
		closeOldPage = true
	}

	index.Offset[(item.Id-1)%IndexCount] = Offset{
		offset: dataHead,
		length: l + 12,
	}

	if err = binary.Write(data, binary.NativeEndian, item.Id); err != nil {
		goto _EXIT
	}

	if err = binary.Write(data, binary.NativeEndian, l); err != nil {
		goto _EXIT
	}

	if _, err = data.Write(item.Data); err != nil {
		goto _EXIT
	}

	m.l.Lock()
	if closeOldPage {
		m.index = index
		m.data = data
	}
	m.dataHead = dataHead + l + 12
	*m.state.Head = item.Id

	for _, notif := range m.notifyItem {
		close(notif)
	}
	m.notifyItem = nil
	m.l.Unlock()

	if closeOldPage {
		err = syscall.Munmap(prevIndex.Data)
		if e := prevIndex.File.Close(); err == nil && e != nil {
			err = e
		}
		if e := prevData.Close(); err == nil && e != nil {
			err = e
		}

		if err != nil {
			goto _EXIT
		}
	}
_EXIT:
	item.Ready <- err
	if callback != nil {
		callback(item)
	}
	return
}

func (c *Cursor) Next(ctx context.Context) (io.ReadCloser, int, error) {
	var start Offset
	var waitForItem chan struct{}
	retLength := 0

	offsetPage := c.head / IndexCount // next item is c.head + 1

	c.memfile.l.Lock()
	pubHead := *c.memfile.state.Head

	if pubHead > c.head {
		// has item
		indexPage := (pubHead - 1) / IndexCount
		indexStart := indexPage*IndexCount + 1

		if offsetPage == indexPage {
			start = c.memfile.index.Offset[int(c.head+1-indexStart)]
			end := c.memfile.index.Offset[int(pubHead-indexStart)]
			retLength = int(end.offset + end.length - start.offset)
		}
	} else {
		waitForItem = make(chan struct{})
		c.memfile.notifyItem = append(c.memfile.notifyItem, waitForItem)
	}

	c.memfile.l.Unlock()

	if waitForItem != nil {
		select {
		case <-ctx.Done():
			return nil, 0, context.Canceled
		case <-waitForItem:
			return nil, 0, nil
		}
	}

	if retLength == 0 {
		// requested page is not current page
		if c.dataFile != nil {
			c.dataFile.Close()
			c.dataFile = nil
		}

		f, err := os.OpenFile(path.Join(c.memfile.indexPath, strconv.Itoa(int(offsetPage))), os.O_RDONLY, 0400)
		if err != nil {
			return nil, 0, err
		}

		if _, err = f.Seek(int64(8*(c.head-offsetPage*IndexCount)), 0); err != nil {
			f.Close()
			return nil, 0, err
		}

		err = binary.Read(f, binary.NativeEndian, &start.offset)
		f.Close()

		if err != nil {
			return nil, 0, err
		}

		if f, err = os.OpenFile(path.Join(c.memfile.dataPath, strconv.Itoa(int(offsetPage))), os.O_RDONLY, 0400); err != nil {
			return nil, 0, err
		}

		if _, err = f.Seek(int64(start.offset), 0); err != nil {
			f.Close()
			return nil, 0, err
		}

		// add all to tracker
		c.lock.Lock()
		for i := c.head + 1; i <= (offsetPage+1)*IndexCount; i++ {
			c.trackerMap[i] = c.trackerList.PushBack(i)
		}
		c.lock.Unlock()

		c.head = (offsetPage + 1) * IndexCount
		return f, 0, nil
	}

	if c.head%IndexCount == 0 {
		// move to a new page
		if c.dataFile != nil {
			c.dataFile.Close()
			c.dataFile = nil
		}
	}

	if c.dataFile == nil {
		f, err := os.OpenFile(path.Join(c.memfile.dataPath, strconv.Itoa(int(offsetPage))), os.O_RDONLY, 0400)
		if err != nil {
			return nil, 0, err
		}

		if _, err = f.Seek(int64(start.offset), 0); err != nil {
			f.Close()
			return nil, 0, err
		}
		c.dataFile = f
	}

	// add all to tracker
	c.lock.Lock()
	for i := c.head + 1; i <= pubHead; i++ {
		c.trackerMap[i] = c.trackerList.PushBack(i)
	}
	c.lock.Unlock()
	c.head = pubHead
	return c.dataFile, retLength, nil
}

func (c *Cursor) Ack(id uint64) {
	c.lock.Lock()
	if i, ok := c.trackerMap[id]; !ok {
		c.lock.Unlock()
		return
	} else {
		delete(c.trackerMap, id)

		if i != c.trackerList.Front() {
			c.trackerList.Remove(i)
			c.lock.Unlock()
			return
		}

		if i.Next() != nil {
			*c.offset = i.Next().Value.(uint64) - 1
		} else {
			*c.offset = id
		}
		c.trackerList.Remove(i)
		c.lock.Unlock()
	}
}

func (c *Cursor) Close() error {
	if c.dataFile != nil {
		return c.dataFile.Close()
	}

	return nil
}
