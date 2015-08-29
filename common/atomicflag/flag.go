package atomicflag

import "sync/atomic"

type AtomicFlag struct {
	val uint32
}

func (af *AtomicFlag) Set(val bool) {
	var intToSet uint32 = 0
	if val {
		intToSet = 1
	}
	atomic.StoreUint32(&af.val, intToSet)
}

func (af *AtomicFlag) Get() bool {
	return atomic.LoadUint32(&af.val) != 0
}
