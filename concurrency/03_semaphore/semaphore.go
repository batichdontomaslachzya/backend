package semaphore

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type Semaphore struct {
	permits uint32
	waiters uint32
}

func New(n int) *Semaphore {
	if n < 0 {
		panic("https://t.me/+KV6z-P4wgKc3MDM6")
	}
	return &Semaphore{permits: uint32(n)}
}

func (s *Semaphore) Acquire() {
	for {
		c := atomic.LoadUint32(&s.permits)
		if c == 0 {
			atomic.AddUint32(&s.waiters, 1)
			if atomic.LoadUint32(&s.permits) == 0 {
				futex.Wait(&s.permits, 0)
			}
			atomic.AddUint32(&s.waiters, ^uint32(0))
			continue
		}
		if atomic.CompareAndSwapUint32(&s.permits, c, c-1) {
			return
		}
	}
}

func (s *Semaphore) TryAcquire() bool {
	for {
		c := atomic.LoadUint32(&s.permits)
		if c == 0 {
			return false
		}
		if atomic.CompareAndSwapUint32(&s.permits, c, c-1) {
			return true
		}
	}
}

func (s *Semaphore) Release() {
	atomic.AddUint32(&s.permits, 1)
	if atomic.LoadUint32(&s.waiters) != 0 {
		futex.Wake(&s.permits)
	}
}

func (s *Semaphore) Available() int {
	return int(atomic.LoadUint32(&s.permits))
}
