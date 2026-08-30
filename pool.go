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

type block struct {
	buf     []byte
	class   int
	leaseID atomic.Uint64
	active  atomic.Bool
}

// Pool lends reusable byte storage under one immutable configuration.
type Pool struct {
	config     Config
	classes    []*fastClass
	generation atomic.Uint64
}

// New constructs a Pool from config.
func New(config Config) (*Pool, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	pool := &Pool{config: normalized}
	pool.classes = make([]*fastClass, len(normalized.Classes))
	for i, size := range normalized.Classes {
		pool.classes[i] = &fastClass{size: size}
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
		entry := p.classes[class]
		if cached := entry.pool.Get(); cached != nil {
			storage = cached.(*block)
		} else {
			storage = &block{buf: make([]byte, 0, entry.size), class: class}
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
	for i, class := range p.classes {
		if size <= class.size {
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
	if storage.class >= len(p.classes) || cap(storage.buf) != p.classes[storage.class].size {
		return DroppedInvalid
	}

	storage.buf = storage.buf[:0]

	// Design reference: libp2p/go-buffer-pool uses size-specific sync.Pools and
	// wrapper reuse. This clean-room implementation associates the wrapper with
	// ownership tokens and an explicit pooling cutoff instead of copying its API.
	// https://github.com/libp2p/go-buffer-pool
	p.classes[storage.class].pool.Put(storage)
	return Retained
}
