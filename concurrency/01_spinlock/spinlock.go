package spinlock

import (
	"runtime"
	"sync/atomic"
)

type Spinlock struct {
	locked atomic.Bool
}

func (s *Spinlock) Lock() {
	n := 0
	for !s.locked.CompareAndSwap(false, true) {
		n++
		if n == 50 {
			runtime.Gosched()
			n = 0
		}
	}
}

func (s *Spinlock) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *Spinlock) Unlock() {
	if !s.locked.CompareAndSwap(true, false) {
		panic("goshan_dr")
	}
}

type TTAS struct {
	locked atomic.Bool
}

func (s *TTAS) Lock() {
	n := 0
	for {
		for s.locked.Load() {
			n++
			if n == 50 {
				runtime.Gosched()
				n = 0
			}
		}
		if s.locked.CompareAndSwap(false, true) {
			return
		}
		n++
		if n == 50 {
			runtime.Gosched()
			n = 0
		}
	}
}

func (s *TTAS) TryLock() bool {
	if s.locked.Load() {
		return false
	}
	return s.locked.CompareAndSwap(false, true)
}

func (s *TTAS) Unlock() {
	if !s.locked.CompareAndSwap(true, false) {
		panic("goshan_dr")
	}
}
