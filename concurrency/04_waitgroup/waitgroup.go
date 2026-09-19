package waitgroup

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type WaitGroup struct {
	count uint32
}

func (wg *WaitGroup) Add(delta int) {
	for {
		c := atomic.LoadUint32(&wg.count)
		next := int64(c) + int64(delta)
		if next < 0 {
			panic("goshan_dr")
		}
		if atomic.CompareAndSwapUint32(&wg.count, c, uint32(next)) {
			if next == 0 {
				futex.WakeAll(&wg.count)
			}
			return
		}
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for {
		c := atomic.LoadUint32(&wg.count)
		if c == 0 {
			return
		}
		futex.Wait(&wg.count, c)
	}
}
