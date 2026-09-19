package rwmutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const writer = 1 << 31

type RWMutex struct {
	state uint32
}

func (rw *RWMutex) RLock() {
	for {
		s := atomic.LoadUint32(&rw.state)
		if s&writer != 0 {
			futex.Wait(&rw.state, s)
			continue
		}
		if atomic.CompareAndSwapUint32(&rw.state, s, s+1) {
			return
		}
	}
}

func (rw *RWMutex) RUnlock() {
	for {
		s := atomic.LoadUint32(&rw.state)
		if s == 0 || s&writer != 0 {
			panic("goshan_dr")
		}
		if atomic.CompareAndSwapUint32(&rw.state, s, s-1) {
			if s-1 == 0 {
				futex.WakeAll(&rw.state)
			}
			return
		}
	}
}

func (rw *RWMutex) Lock() {
	for {
		if atomic.CompareAndSwapUint32(&rw.state, 0, writer) {
			return
		}
		s := atomic.LoadUint32(&rw.state)
		if s != 0 {
			futex.Wait(&rw.state, s)
		}
	}
}

func (rw *RWMutex) Unlock() {
	if !atomic.CompareAndSwapUint32(&rw.state, writer, 0) {
		panic("goshan_dr")
	}
	futex.WakeAll(&rw.state)
}
