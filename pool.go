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
	idle []*backingStorage
}

type backingStorage struct {
	buf     []byte
	class   int
	leaseID atomic.Uint64
	active  atomic.Bool
}

type poolGeneration struct {
	id               uint64
	fastClasses      []*fastClass
	boundedClasses   []*boundedClass
	retainedCapacity atomic.Int64
}

// Pool lends reusable byte storage under one immutable configuration.
type Pool struct {
	config       Config
	sizes        []int
	clearMu      sync.Mutex
	generation   atomic.Uint64
	current      atomic.Pointer[poolGeneration]
	rawWrappers  sync.Pool
	validationMu sync.Mutex
	rawRecords   map[uintptr]rawRecord
	counters     *poolCounters
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
	if normalized.ValidationEnabled {
		pool.rawRecords = make(map[uintptr]rawRecord)
	}
	if normalized.StatsEnabled {
		pool.counters = newPoolCounters(normalized.Classes)
	}
	pool.current.Store(pool.newGeneration(0))
	return pool, nil
}

func (p *Pool) newGeneration(id uint64) *poolGeneration {
	generation := &poolGeneration{id: id}
	if p.config.Mode == Fast {
		generation.fastClasses = make([]*fastClass, len(p.sizes))
		for i, size := range p.sizes {
			generation.fastClasses[i] = &fastClass{size: size}
		}
	} else {
		generation.boundedClasses = make([]*boundedClass, len(p.sizes))
		for i, size := range p.sizes {
			generation.boundedClasses[i] = &boundedClass{size: size}
		}
	}
	return generation
}

// Clear discards currently idle Backing Storage and advances Pool Generation.
func (p *Pool) Clear() {
	p.clearMu.Lock()
	id := p.generation.Add(1)
	p.current.Store(p.newGeneration(id))
	p.clearMu.Unlock()
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
	if err := p.validateSize(size); err != nil {
		return Lease{}, err
	}

	storage, generation := p.acquireStorage(size)
	token := storage.leaseID.Add(1)
	storage.active.Store(true)
	return Lease{
		pool:       p,
		storage:    storage,
		token:      token,
		generation: generation.id,
	}, nil
}

func (p *Pool) validateSize(size int) error {
	if size < 0 {
		return fmt.Errorf("%w: negative size %d", ErrInvalidSize, size)
	}
	if p.config.MaxAcquireSize > 0 && size > p.config.MaxAcquireSize {
		return fmt.Errorf("%w: size %d exceeds limit %d", ErrInvalidSize, size, p.config.MaxAcquireSize)
	}
	return nil
}

func (p *Pool) acquireStorage(size int) (*backingStorage, *poolGeneration) {
	generation := p.current.Load()
	class := p.classForSize(size)
	var storage *backingStorage
	hit := false
	if class >= 0 {
		if p.config.Mode == Fast {
			entry := generation.fastClasses[class]
			if cached := entry.pool.Get(); cached != nil {
				storage = cached.(*backingStorage)
				hit = true
			} else {
				storage = &backingStorage{buf: make([]byte, 0, entry.size), class: class}
			}
		} else {
			entry := generation.boundedClasses[class]
			entry.mu.Lock()
			if last := len(entry.idle) - 1; last >= 0 {
				generation.retainedCapacity.Add(-int64(entry.size))
				storage = entry.idle[last]
				entry.idle[last] = nil
				entry.idle = entry.idle[:last]
			}
			entry.mu.Unlock()
			if storage != nil {
				hit = true
			} else {
				storage = &backingStorage{buf: make([]byte, 0, entry.size), class: class}
			}
		}
		storage.buf = storage.buf[:size]
	} else {
		storage = &backingStorage{buf: make([]byte, size), class: -1}
	}
	p.recordAcquire(class, hit)
	return storage, generation
}

func (p *Pool) classForSize(size int) int {
	for i, classSize := range p.sizes {
		if size <= classSize {
			return i
		}
	}
	return -1
}

func (p *Pool) classForCapacity(capacity int) int {
	for i, classSize := range p.sizes {
		if capacity == classSize {
			return i
		}
	}
	return -1
}

func (p *Pool) release(storage *backingStorage, leaseGeneration uint64) ReleaseStatus {
	class := storage.class
	generation := p.current.Load()
	if leaseGeneration != generation.id {
		return p.recordRelease(DroppedStale, class)
	}
	if cap(storage.buf) > p.config.MaxPooledCapacity {
		return p.recordRelease(DroppedOversize, class)
	}
	if class < 0 {
		return p.recordRelease(DroppedInvalid, class)
	}
	if class >= len(p.sizes) || cap(storage.buf) != p.sizes[class] {
		return p.recordRelease(DroppedInvalid, class)
	}

	storage.buf = storage.buf[:0]
	if p.config.Mode == Bounded {
		return p.recordRelease(p.releaseBounded(generation, storage), class)
	}

	// Design reference: libp2p/go-buffer-pool uses size-specific sync.Pools and
	// wrapper reuse. This clean-room implementation associates the wrapper with
	// ownership tokens and an explicit pooling cutoff instead of copying its API.
	// https://github.com/libp2p/go-buffer-pool
	generation.fastClasses[class].pool.Put(storage)
	return p.recordRelease(Retained, class)
}

func (p *Pool) releaseBounded(generation *poolGeneration, storage *backingStorage) ReleaseStatus {
	capacity := int64(cap(storage.buf))
	for {
		retained := generation.retainedCapacity.Load()
		if retained+capacity > p.config.MaxRetainedCapacity {
			return DroppedFull
		}
		if generation.retainedCapacity.CompareAndSwap(retained, retained+capacity) {
			break
		}
	}

	// Design reference: oxtoacart/bpool drops releases when its bounded
	// container is full. This implementation uses a global byte-capacity CAS
	// plus per-class LIFO lists instead of copying its channel-based design.
	// https://github.com/oxtoacart/bpool
	entry := generation.boundedClasses[storage.class]
	entry.mu.Lock()
	entry.idle = append(entry.idle, storage)
	entry.mu.Unlock()
	return Retained
}
