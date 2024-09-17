package stream

import (
	"container/list"
	"sync"
)

type Transition[TState any, TMessage any, TDelta any] func(state TState, message TMessage) (TState, TDelta, error)

type _StreamState[TState any, TDelta any] struct {
	isCurrent bool
	count     int

	state TState
	delta TDelta

	cond *sync.Cond
}

type Stream[TState any, TMessage any, TDelta any] struct {
	lock sync.Mutex

	transition Transition[TState, TMessage, TDelta]
	list       *list.List
}

func (s *Stream[TState, TMessage, TDelta]) Init(transition Transition[TState, TMessage, TDelta], initState TState) {
	s.transition = transition
	s.list = list.New()

	var l sync.Mutex
	s.list.PushBack(&_StreamState[TState, TDelta]{
		state:     initState,
		isCurrent: true,
		cond:      sync.NewCond(&l),
	})
}

// Send a message to the stream.
func (s *Stream[TState, TMessage, TDelta]) Send(msg TMessage) error {
	s.lock.Lock()
	curr := s.list.Back().Value.(*_StreamState[TState, TDelta])
	s.lock.Unlock()

	state, delta, err := s.transition(curr.state, msg)
	if err != nil {
		return err
	}

	s.lock.Lock()

	if s.list.Len() == 1 && curr.count == 0 {
		curr.state = state
		curr.delta = delta
		s.lock.Unlock()
		return nil
	}

	curr.isCurrent = false

	var l sync.Mutex
	s.list.PushBack(&_StreamState[TState, TDelta]{
		state:     state,
		delta:     delta,
		isCurrent: true,
		cond:      sync.NewCond(&l),
	})

	s.lock.Unlock()
	curr.cond.Broadcast()

	return nil
}

type Snapshot[TState any, TMessage any, TDelta any] struct {
	stream    *Stream[TState, TMessage, TDelta]
	curr      *list.Element
	isWaiting bool
	isClosed  bool
}

func (s *Stream[TState, TMessage, TDelta]) CreateSnapshot() Snapshot[TState, TMessage, TDelta] {
	s.lock.Lock()
	defer s.lock.Unlock()

	curr := s.list.Back()
	curr.Value.(*_StreamState[TState, TDelta]).count++

	return Snapshot[TState, TMessage, TDelta]{
		stream: s,
		curr:   curr,
	}
}

func (s *Snapshot[TState, TMessage, TDelta]) GetState() TState {
	s.stream.lock.Lock()
	defer s.stream.lock.Unlock()
	return s.curr.Value.(*_StreamState[TState, TDelta]).state
}

func (s *Snapshot[TState, TMessage, TDelta]) tryNext(lockForNext bool) (ret *_StreamState[TState, TDelta], ok bool) {
	s.stream.lock.Lock()
	defer s.stream.lock.Unlock()

	if s.isClosed {
		return
	}

	ret = s.curr.Value.(*_StreamState[TState, TDelta])

	if ret.isCurrent {
		if lockForNext {
			ret.cond.L.Lock()
			s.isWaiting = true
		}
		return
	}

	next := s.curr.Next()
	ret.count--
	if ret.count == 0 && s.curr == s.stream.list.Front() {
		s.stream.list.Remove(s.curr)
	}

	s.curr = next
	ret = next.Value.(*_StreamState[TState, TDelta])
	ret.count++

	ok = true
	return
}

func (s *Snapshot[TState, TMessage, TDelta]) TryNext() (delta TDelta, ok bool) {
	n, ok := s.tryNext(false)
	if !ok {
		return
	}

	return n.delta, ok
}

func (s *Snapshot[TState, TMessage, TDelta]) Next() (delta TDelta, closed bool) {
	n, ok := s.tryNext(true)
	for !ok {
		if n == nil {
			closed = true
			return
		}

		n.cond.Wait()
		s.isWaiting = false
		n.cond.L.Unlock()
		n, ok = s.tryNext(true)
	}

	return n.delta, false
}

func (s *Snapshot[TState, TMessage, TDelta]) Close() {
	s.stream.lock.Lock()

	if s.isClosed {
		s.stream.lock.Unlock()
		return
	}

	s.isClosed = true

	curr := s.curr.Value.(*_StreamState[TState, TDelta])
	curr.count--

	next := s.curr
	value := curr
	for value.count == 0 && !value.isCurrent && next == s.stream.list.Front() {
		s.stream.list.Remove(next)

		next = next.Next()
		value = next.Value.(*_StreamState[TState, TDelta])
	}

	s.stream.lock.Unlock()

	curr.cond.L.Lock()
	if s.isWaiting {
		curr.cond.Broadcast()
	}
	curr.cond.L.Unlock()
}
