package lib

import "sync"

type noCopy struct{}

type Semaphore struct {
	noCopy noCopy
	c      *sync.Cond
	Value  int
}

func NewSemaphore(n int) *Semaphore {
	return &Semaphore{
		c:     sync.NewCond(&sync.Mutex{}),
		Value: n,
	}
}

func (s *Semaphore) TryAcquire(n int) bool {
	if !s.c.L.(*sync.Mutex).TryLock() {
		return false
	}

	if s.Value < n {
		s.c.L.Unlock()
		return false
	}

	s.Value -= n
	s.c.L.Unlock()
	return true
}

func (s *Semaphore) Acquire(n int) {
	s.c.L.Lock()
	for s.Value < n {
		s.c.Wait()
	}

	s.Value -= n
	s.c.L.Unlock()
}

func (s *Semaphore) Release(n int) {
	s.c.L.Lock()
	s.Value += n
	s.c.L.Unlock()
	s.c.Broadcast()
}

func (s *Semaphore) Reset(n int) {
	s.c.L.Lock()
	s.Value = n
	s.c.L.Unlock()
	s.c.Broadcast()
}

type Pool[T any] struct {
	new     func() T
	cap     int
	items   []T
	created int
	c       *sync.Cond
}

func NewPool[T any](new func() T, cap int) (ret Pool[T]) {
	ret.new = new
	ret.cap = cap
	ret.c = sync.NewCond(&sync.Mutex{})
	ret.items = make([]T, 0, cap)
	return
}

// func (p *Pool[T]) Init(cap int) {
// 	p.c = sync.NewCond(&sync.Mutex{})
// 	p.items = make([]T, 0, cap)
// }

func (p *Pool[T]) Get() (ret T) {
	p.c.L.Lock()
	for {
		l := len(p.items)
		if l > 0 {
			ret = p.items[l-1]
			p.items = p.items[:l-1]
			p.c.L.Unlock()
			return
		}

		if p.created < p.cap {
			ret = p.new()
			p.c.L.Unlock()
			return
		}

		p.c.Wait()
	}
}

func (p *Pool[T]) Put(item T) {
	p.c.L.Lock()
	p.items = append(p.items, item)
	p.c.L.Unlock()
	p.c.Signal()
}
