package once

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type Once struct {
	state uint32
}

func (o *Once) Do(f func()) {
	if atomic.LoadUint32(&o.state) == 2 {
		return
	}
	if atomic.CompareAndSwapUint32(&o.state, 0, 1) {
		defer func() {
			atomic.StoreUint32(&o.state, 2)
			futex.WakeAll(&o.state)
		}()
		f()
		return
	}
	for {
		s := atomic.LoadUint32(&o.state)
		if s == 2 {
			return
		}
		futex.Wait(&o.state, 1)
	}
}

func (o *Once) Done() bool {
	return atomic.LoadUint32(&o.state) == 2
}
