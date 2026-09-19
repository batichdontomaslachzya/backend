package mutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const (
	free = iota
	held
	contended
)

type Mutex struct {
	state uint32
}

func (m *Mutex) Lock() {
	if atomic.CompareAndSwapUint32(&m.state, free, held) {
		return
	}
	for {
		if atomic.SwapUint32(&m.state, contended) == free {
			return
		}
		futex.Wait(&m.state, contended)
	}
}

func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.state, free, held)
}

func (m *Mutex) Unlock() {
	o := atomic.SwapUint32(&m.state, free)
	if o == free {
		panic("goshan_dr")
	}
	if o == contended {
		futex.Wake(&m.state)
	}
}
