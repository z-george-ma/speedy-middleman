package lib

import (
	"sync/atomic"
	"time"
)

type Monotonic struct {
	lastTime atomic.Int64
	hostBits int
	hostId   int
}

func (m *Monotonic) Init(lastTime int64, hostBits int, hostId int) {
	m.lastTime.Store(lastTime)
	m.hostBits = hostBits
	m.hostId = hostId
}

func (m *Monotonic) Value() int64 {
	now := (time.Now().UnixNano()>>int64(m.hostBits))<<int64(m.hostBits) + int64(m.hostId)

	for {
		lastTime := m.lastTime.Load()

		if now <= lastTime {
			now = (lastTime>>int64(m.hostBits)+1)<<int64(m.hostBits) + int64(m.hostId)
		}

		if m.lastTime.CompareAndSwap(lastTime, now) {
			return now
		}
	}
}
