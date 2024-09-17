package lib

import (
	"context"
	"sync"
)

type Queue[T any] struct {
	Values           []T // access to Values is not thread safe
	notifyCh         chan struct{}
	isNotifyChClosed bool
	lock             sync.Mutex
}

func (q *Queue[T]) Init(size ...int) {
	if len(size) > 0 {
		q.Values = make([]T, 0, size[0])
	}

	q.refreshNotifyChannel()
}

func (q *Queue[T]) Push(item T) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.Values = append(q.Values, item)
	if !q.isNotifyChClosed {
		close(q.notifyCh)
		q.isNotifyChClosed = true
	}
}

func (q *Queue[T]) first() (ret T, ok bool) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if len(q.Values) > 0 {
		return q.Values[0], true
	}
	return
}

func (q *Queue[T]) refreshNotifyChannel() {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.notifyCh = make(chan struct{})
	q.isNotifyChClosed = false
}

func (q *Queue[T]) Peek(ctx context.Context) (ret T, ok bool) {
	ret, ok = q.first()

	if ok {
		return
	}

	select {
	case <-q.notifyCh:
		q.refreshNotifyChannel()
		return q.first()
	case <-ctx.Done():
		return ret, false
	}
}

func (q *Queue[T]) Pop() {
	q.lock.Lock()
	defer q.lock.Unlock()
	if len(q.Values) > 0 {
		q.Values = q.Values[1:]
	}
}
