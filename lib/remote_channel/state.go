package remotechannel

import (
	"errors"
	"os"
	"sync"
	"unsafe"
)

type SubscriptionLayout struct {
	Head     uint64
	KeySize  int // key size
	KeyStart byte
}

type Subscription struct {
	Key  string
	Head *uint64
}

type StateLayout struct {
	EarlistPage uint64
	Head        uint64
	SubSize     int
	SubStart    byte
}

type State struct {
	Data         []byte
	File         *os.File
	EarliestPage *uint64
	Head         *uint64
	SubSize      *int
	Lock         sync.Mutex
	Sub          []*Subscription // read-only
	End          unsafe.Pointer
}

func (state *State) Init(mem []byte, shouldInit bool) {
	s := (*StateLayout)(unsafe.Pointer(&mem[0]))

	if shouldInit {
		s.EarlistPage = 0
		s.Head = 0
		s.SubSize = 0
	}

	state.EarliestPage = &s.EarlistPage
	state.Head = &s.Head
	state.SubSize = &s.SubSize

	p := unsafe.Pointer(&s.SubStart)
	for i := 0; i < s.SubSize; i++ {
		sub := (*SubscriptionLayout)(p)
		state.Sub = append(state.Sub, &Subscription{
			Head: &sub.Head,
			Key:  unsafe.String(&sub.KeyStart, sub.KeySize),
		})
		p = unsafe.Add(p, 16+sub.KeySize)
	}
	state.End = p
}

func (state *State) AddSub(key string, head uint64) (*Subscription, error) {
	if uintptr(state.End)+uintptr(16+len(key)) > uintptr(unsafe.Pointer(&state.Data[0]))+uintptr(stateFileSize) {
		return nil, errors.New("Max subscriptions reached")
	}
	sub := (*SubscriptionLayout)(state.End)
	sub.Head = head
	sub.KeySize = len(key)
	s := unsafe.Slice(&sub.KeyStart, sub.KeySize)
	copy(s, unsafe.Slice(unsafe.StringData(key), sub.KeySize))

	ret := &Subscription{
		Head: &sub.Head,
		Key:  unsafe.String(&sub.KeyStart, sub.KeySize),
	}

	state.Sub = append(state.Sub, ret)
	*state.SubSize++
	state.End = unsafe.Add(state.End, 16+sub.KeySize)

	return ret, nil
}

func (state *State) GetOrAddSub(key string, head uint64) (*uint64, uint64, error) {
	state.Lock.Lock()
	defer state.Lock.Unlock()

	for _, s := range state.Sub {
		if s.Key == key {
			return s.Head, *s.Head, nil
		}
	}

	if r, err := state.AddSub(key, head); err != nil {
		return nil, 0, err
	} else {
		return r.Head, *r.Head, nil
	}
}
