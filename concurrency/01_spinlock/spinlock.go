package spinlock

import "sync/atomic"

type Spinlock struct {
	locked atomic.Bool
}

func (s *Spinlock) Lock() {
	for !s.locked.CompareAndSwap(false, true) {
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
	for {
		for s.locked.Load() {
		}
		if s.locked.CompareAndSwap(false, true) {
			return
		}
	}
}

func (s *TTAS) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *TTAS) Unlock() {
	if !s.locked.CompareAndSwap(true, false) {
		panic("goshan_dr")
	}
}
