package rwmutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const (
	writer   = 1 << 31
	waiting  = 1 << 24
	waitMask = 0x7f << 24
	sleeping = 1 << 23
	readers  = sleeping - 1
)

type RWMutex struct {
	state uint32
}

func (rw *RWMutex) RLock() {
	for {
		s := atomic.LoadUint32(&rw.state)
		if s&writer != 0 || s&waitMask != 0 {
			if s&sleeping == 0 {
				if !atomic.CompareAndSwapUint32(&rw.state, s, s|sleeping) {
					continue
				}
				s |= sleeping
			}
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
		if s&writer != 0 || s&readers == 0 {
			panic("goshan_dr")
		}
		if atomic.CompareAndSwapUint32(&rw.state, s, s-1) {
			n := s - 1
			if n&readers == 0 && n&sleeping != 0 {
				for {
					x := atomic.LoadUint32(&rw.state)
					if x&sleeping == 0 {
						break
					}
					if atomic.CompareAndSwapUint32(&rw.state, x, x&^sleeping) {
						futex.WakeAll(&rw.state)
						break
					}
				}
			}
			return
		}
	}
}

func (rw *RWMutex) Lock() {
	for {
		s := atomic.LoadUint32(&rw.state)
		if atomic.CompareAndSwapUint32(&rw.state, s, s+waiting) {
			break
		}
	}
	for {
		s := atomic.LoadUint32(&rw.state)
		if s&writer == 0 && s&readers == 0 {
			n := (s - waiting) | writer
			if atomic.CompareAndSwapUint32(&rw.state, s, n) {
				return
			}
			continue
		}
		if s&sleeping == 0 {
			if !atomic.CompareAndSwapUint32(&rw.state, s, s|sleeping) {
				continue
			}
			s |= sleeping
		}
		futex.Wait(&rw.state, s)
	}
}

func (rw *RWMutex) Unlock() {
	for {
		s := atomic.LoadUint32(&rw.state)
		if s&writer == 0 {
			panic("goshan_dr")
		}
		n := s &^ writer
		if n&sleeping != 0 {
			n &^= sleeping
		}
		if atomic.CompareAndSwapUint32(&rw.state, s, n) {
			if s&sleeping != 0 {
				futex.WakeAll(&rw.state)
			}
			return
		}
	}
}
