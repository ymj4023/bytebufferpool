package bytebufferpool

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type fastClass struct {
	size int
	pool sync.Pool
}

type boundedClass struct {
	size int
	mu   sync.Mutex
	free []*block
}

type block struct {
	buf     []byte
	class   int
	leaseID atomic.Uint64
	active  atomic.Bool
}

// Pool lends reusable byte storage under one immutable configuration.
type Pool struct {
	config          Config
	sizes           []int
	fastClasses     []*fastClass
	boundedClasses  []*boundedClass
	generation      atomic.Uint64
	retainedBuffers atomic.Int64
	retainedBytes   atomic.Int64
}

// New constructs a Pool from config.
func New(config Config) (*Pool, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	pool := &Pool{
		config: normalized,
		sizes:  append([]int(nil), normalized.Classes...),
	}
	for i, size := range normalized.Classes {
		if normalized.Mode == Fast {
			if pool.fastClasses == nil {
				pool.fastClasses = make([]*fastClass, len(normalized.Classes))
			}
			pool.fastClasses[i] = &fastClass{size: size}
		} else {
			if pool.boundedClasses == nil {
				pool.boundedClasses = make([]*boundedClass, len(normalized.Classes))
			}
			pool.boundedClasses[i] = &boundedClass{size: size}
		}
	}
	return pool, nil
}

// Acquire returns a Lease of size bytes and panics when size is invalid.
func (p *Pool) Acquire(size int) Lease {
	lease, err := p.TryAcquire(size)
	if err != nil {
		panic(err)
	}
	return lease
}

// TryAcquire returns a Lease of size bytes.
func (p *Pool) TryAcquire(size int) (Lease, error) {
	if size < 0 {
		return Lease{}, fmt.Errorf("%w: negative size %d", ErrInvalidSize, size)
	}
	if p.config.MaxAcquireSize > 0 && size > p.config.MaxAcquireSize {
		return Lease{}, fmt.Errorf("%w: size %d exceeds limit %d", ErrInvalidSize, size, p.config.MaxAcquireSize)
	}

	class := p.classForSize(size)
	var storage *block
	if class >= 0 {
		if p.config.Mode == Fast {
			entry := p.fastClasses[class]
			if cached := entry.pool.Get(); cached != nil {
				storage = cached.(*block)
			} else {
				storage = &block{buf: make([]byte, 0, entry.size), class: class}
			}
		} else {
			entry := p.boundedClasses[class]
			entry.mu.Lock()
			if last := len(entry.free) - 1; last >= 0 {
				storage = entry.free[last]
				entry.free[last] = nil
				entry.free = entry.free[:last]
			}
			entry.mu.Unlock()
			if storage != nil {
				p.retainedBuffers.Add(-1)
				p.retainedBytes.Add(-int64(entry.size))
			} else {
				storage = &block{buf: make([]byte, 0, entry.size), class: class}
			}
		}
		storage.buf = storage.buf[:size]
	} else {
		storage = &block{buf: make([]byte, size), class: -1}
	}

	token := storage.leaseID.Add(1)
	storage.active.Store(true)
	return Lease{
		pool:       p,
		storage:    storage,
		token:      token,
		generation: p.generation.Load(),
	}, nil
}

func (p *Pool) classForSize(size int) int {
	for i, classSize := range p.sizes {
		if size <= classSize {
			return i
		}
	}
	return -1
}

func (p *Pool) release(storage *block) ReleaseStatus {
	if cap(storage.buf) > p.config.MaxPooledCapacity {
		return DroppedOversize
	}
	if storage.class < 0 {
		return DroppedInvalid
	}
	if storage.class >= len(p.sizes) || cap(storage.buf) != p.sizes[storage.class] {
		return DroppedInvalid
	}

	storage.buf = storage.buf[:0]
	if p.config.Mode == Bounded {
		return p.releaseBounded(storage)
	}

	// Design reference: libp2p/go-buffer-pool uses size-specific sync.Pools and
	// wrapper reuse. This clean-room implementation associates the wrapper with
	// ownership tokens and an explicit pooling cutoff instead of copying its API.
	// https://github.com/libp2p/go-buffer-pool
	p.fastClasses[storage.class].pool.Put(storage)
	return Retained
}

func (p *Pool) releaseBounded(storage *block) ReleaseStatus {
	capacity := int64(cap(storage.buf))
	for {
		retained := p.retainedBytes.Load()
		if retained+capacity > p.config.MaxRetainedBytes {
			return DroppedFull
		}
		if p.retainedBytes.CompareAndSwap(retained, retained+capacity) {
			break
		}
	}

	// Design reference: oxtoacart/bpool drops releases when its bounded
	// container is full. This implementation uses a global byte-capacity CAS
	// plus per-class LIFO lists instead of copying its channel-based design.
	// https://github.com/oxtoacart/bpool
	entry := p.boundedClasses[storage.class]
	entry.mu.Lock()
	entry.free = append(entry.free, storage)
	entry.mu.Unlock()
	p.retainedBuffers.Add(1)
	return Retained
}
