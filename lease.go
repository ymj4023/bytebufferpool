package bytebufferpool

import "fmt"

// Lease grants exclusive, generation-bound ownership of Backing Storage.
// A Lease must not be copied after first use.
type Lease struct {
	self       *Lease
	pool       *Pool
	storage    *block
	token      uint64
	generation uint64
	released   bool
}

// Bytes returns the bytes owned by the Lease.
func (l *Lease) Bytes() []byte {
	l.checkUsable()
	return l.storage.buf
}

// Len returns the logical length of the Lease.
func (l *Lease) Len() int {
	l.checkUsable()
	return len(l.storage.buf)
}

// Cap returns the Backing Storage capacity of the Lease.
func (l *Lease) Cap() int {
	l.checkUsable()
	return cap(l.storage.buf)
}

// Release transfers Backing Storage ownership back to the Pool.
func (l *Lease) Release() ReleaseStatus {
	l.checkCopy()
	if l.released || l.storage == nil {
		return RejectedDuplicate
	}
	if l.storage.leaseID.Load() != l.token || !l.storage.active.CompareAndSwap(true, false) {
		l.released = true
		return RejectedDuplicate
	}

	status := l.pool.release(l.storage, l.generation)
	l.released = true
	return status
}

func (l *Lease) checkCopy() {
	if l == nil {
		panic("bytebufferpool: nil Lease")
	}
	if l.self == nil {
		l.self = l
	} else if l.self != l {
		panic("bytebufferpool: illegal copy of Lease")
	}
}

func (l *Lease) checkUsable() {
	l.checkCopy()
	if l.released || l.storage == nil {
		panic("bytebufferpool: use of released Lease")
	}
	if l.storage.leaseID.Load() != l.token || !l.storage.active.Load() {
		panic(fmt.Sprintf("bytebufferpool: Lease %d no longer owns Backing Storage", l.token))
	}
}
