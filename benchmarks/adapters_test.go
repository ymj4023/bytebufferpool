package benchmarks

import (
	"bytes"
	"sync"

	libp2ppool "github.com/libp2p/go-buffer-pool"
	"github.com/oxtoacart/bpool"
	prompool "github.com/prometheus/prometheus/util/pool"
	valyalapool "github.com/valyala/bytebufferpool"
	bytebufferpool "github.com/ymj4023/bytebufferpool"
	grpcmem "google.golang.org/grpc/mem"
)

type rawBorrowed struct {
	bytes   []byte
	pointer *[]byte
	lease   bytebufferpool.Lease
}

type rawAdapter interface {
	Name() string
	Acquire(size int, borrowed *rawBorrowed)
	Release(*rawBorrowed)
}

type makeRawAdapter struct{}

func (makeRawAdapter) Name() string { return "Make" }
func (makeRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	borrowed.bytes = make([]byte, size)
}
func (makeRawAdapter) Release(*rawBorrowed) {}

type syncRawAdapter struct {
	name                string
	maxRetainedCapacity int
	pool                sync.Pool
}

func (a *syncRawAdapter) Name() string { return a.name }
func (a *syncRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	var pointer *[]byte
	if cached := a.pool.Get(); cached != nil {
		pointer = cached.(*[]byte)
	} else {
		pointer = new([]byte)
	}
	if cap(*pointer) < size {
		*pointer = make([]byte, size)
	} else {
		*pointer = (*pointer)[:size]
	}
	borrowed.bytes = *pointer
	borrowed.pointer = pointer
}
func (a *syncRawAdapter) Release(borrowed *rawBorrowed) {
	*borrowed.pointer = borrowed.bytes[:0]
	if a.maxRetainedCapacity == 0 || cap(borrowed.bytes) <= a.maxRetainedCapacity {
		a.pool.Put(borrowed.pointer)
	}
}

type projectLeaseAdapter struct {
	name string
	pool *bytebufferpool.Pool
}

func (a *projectLeaseAdapter) Name() string { return a.name }
func (a *projectLeaseAdapter) Acquire(size int, borrowed *rawBorrowed) {
	borrowed.lease = a.pool.Acquire(size)
	borrowed.bytes = borrowed.lease.Bytes()
}
func (*projectLeaseAdapter) Release(borrowed *rawBorrowed) { borrowed.lease.Release() }

type projectRawAdapter struct {
	name string
	pool *bytebufferpool.Pool
}

func (a *projectRawAdapter) Name() string { return a.name }
func (a *projectRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	borrowed.bytes = a.pool.AcquireSlice(size)
}
func (a *projectRawAdapter) Release(borrowed *rawBorrowed) {
	a.pool.ReleaseSlice(borrowed.bytes)
}

type libp2pRawAdapter struct{ pool libp2ppool.BufferPool }

func (*libp2pRawAdapter) Name() string { return "libp2p/v0.1.0" }
func (a *libp2pRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	borrowed.bytes = a.pool.Get(size)
}
func (a *libp2pRawAdapter) Release(borrowed *rawBorrowed) { a.pool.Put(borrowed.bytes) }

type grpcRawAdapter struct{ pool grpcmem.BufferPool }

func (*grpcRawAdapter) Name() string { return "gRPC/v1.83.2/ZeroOnAcquire" }
func (a *grpcRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	pointer := a.pool.Get(size)
	borrowed.bytes = *pointer
	borrowed.pointer = pointer
}
func (a *grpcRawAdapter) Release(borrowed *rawBorrowed) { a.pool.Put(borrowed.pointer) }

type prometheusRawAdapter struct{ pool *prompool.Pool }

func (*prometheusRawAdapter) Name() string { return "Prometheus/v0.314.0" }
func (a *prometheusRawAdapter) Acquire(size int, borrowed *rawBorrowed) {
	buffer := a.pool.Get(size).([]byte)
	borrowed.bytes = buffer[:size]
}
func (a *prometheusRawAdapter) Release(borrowed *rawBorrowed) { a.pool.Put(borrowed.bytes) }

func rawAdapters() []rawAdapter {
	boundedConfig := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
	boundedConfig.MaxRetainedCapacity = 64 << 20
	bounded := mustPool(boundedConfig)
	zeroConfig := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
	zeroConfig.ZeroOnRelease = true
	validatedConfig := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
	validatedConfig.ValidationEnabled = true
	statsConfig := bytebufferpool.DefaultConfig(bytebufferpool.Fast)
	statsConfig.StatsEnabled = true
	grpcPool, err := grpcmem.NewBinaryTieredBufferPool(6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)
	if err != nil {
		panic(err)
	}

	return []rawAdapter{
		makeRawAdapter{},
		&syncRawAdapter{name: "sync.Pool/Naive"},
		&syncRawAdapter{name: "sync.Pool/Cutoff", maxRetainedCapacity: 1 << 20},
		&projectLeaseAdapter{name: "Project/Fast/Lease", pool: mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))},
		&projectRawAdapter{name: "Project/Fast/Raw", pool: mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))},
		&projectRawAdapter{name: "Project/Bounded/Raw", pool: bounded},
		&projectRawAdapter{name: "Project/Fast/RawZero", pool: mustPool(zeroConfig)},
		&projectRawAdapter{name: "Project/Fast/RawValidation", pool: mustPool(validatedConfig)},
		&projectRawAdapter{name: "Project/Fast/RawStats", pool: mustPool(statsConfig)},
		&libp2pRawAdapter{},
		&grpcRawAdapter{pool: grpcPool},
		&prometheusRawAdapter{pool: prompool.New(64, 1<<20, 2, func(size int) any {
			return make([]byte, 0, size)
		})},
	}
}

type bufferBorrowed struct {
	project  bytebufferpool.Buffer
	standard *bytes.Buffer
	valyala  *valyalapool.ByteBuffer
}

type bufferAdapter interface {
	Name() string
	Acquire() bufferBorrowed
	Write(*bufferBorrowed, []byte)
	Bytes(*bufferBorrowed) []byte
	Release(*bufferBorrowed)
}

type newBytesBufferAdapter struct{}

func (newBytesBufferAdapter) Name() string { return "bytes.Buffer/New" }
func (newBytesBufferAdapter) Acquire() bufferBorrowed {
	return bufferBorrowed{standard: &bytes.Buffer{}}
}
func (newBytesBufferAdapter) Write(borrowed *bufferBorrowed, data []byte) {
	_, _ = borrowed.standard.Write(data)
}
func (newBytesBufferAdapter) Bytes(borrowed *bufferBorrowed) []byte {
	return borrowed.standard.Bytes()
}
func (newBytesBufferAdapter) Release(*bufferBorrowed) {}

type syncBytesBufferAdapter struct {
	name                string
	maxRetainedCapacity int
	pool                sync.Pool
}

func (a *syncBytesBufferAdapter) Name() string { return a.name }
func (a *syncBytesBufferAdapter) Acquire() bufferBorrowed {
	if cached := a.pool.Get(); cached != nil {
		return bufferBorrowed{standard: cached.(*bytes.Buffer)}
	}
	return bufferBorrowed{standard: &bytes.Buffer{}}
}
func (a *syncBytesBufferAdapter) Write(borrowed *bufferBorrowed, data []byte) {
	_, _ = borrowed.standard.Write(data)
}
func (a *syncBytesBufferAdapter) Bytes(borrowed *bufferBorrowed) []byte {
	return borrowed.standard.Bytes()
}
func (a *syncBytesBufferAdapter) Release(borrowed *bufferBorrowed) {
	capacity := cap(borrowed.standard.Bytes())
	borrowed.standard.Reset()
	if a.maxRetainedCapacity == 0 || capacity <= a.maxRetainedCapacity {
		a.pool.Put(borrowed.standard)
	}
}

type valyalaBufferAdapter struct{ pool valyalapool.Pool }

func (*valyalaBufferAdapter) Name() string { return "valyala/v1.0.0" }
func (a *valyalaBufferAdapter) Acquire() bufferBorrowed {
	return bufferBorrowed{valyala: a.pool.Get()}
}
func (*valyalaBufferAdapter) Write(borrowed *bufferBorrowed, data []byte) {
	_, _ = borrowed.valyala.Write(data)
}
func (*valyalaBufferAdapter) Bytes(borrowed *bufferBorrowed) []byte {
	return borrowed.valyala.Bytes()
}
func (a *valyalaBufferAdapter) Release(borrowed *bufferBorrowed) {
	a.pool.Put(borrowed.valyala)
}

type bpoolBufferAdapter struct {
	name string
	pool interface {
		Get() *bytes.Buffer
		Put(*bytes.Buffer)
	}
}

func (a *bpoolBufferAdapter) Name() string { return a.name }
func (a *bpoolBufferAdapter) Acquire() bufferBorrowed {
	return bufferBorrowed{standard: a.pool.Get()}
}
func (*bpoolBufferAdapter) Write(borrowed *bufferBorrowed, data []byte) {
	_, _ = borrowed.standard.Write(data)
}
func (*bpoolBufferAdapter) Bytes(borrowed *bufferBorrowed) []byte {
	return borrowed.standard.Bytes()
}
func (a *bpoolBufferAdapter) Release(borrowed *bufferBorrowed) {
	a.pool.Put(borrowed.standard)
}

type projectBufferAdapter struct {
	name string
	pool *bytebufferpool.Pool
}

func (a *projectBufferAdapter) Name() string { return a.name }
func (a *projectBufferAdapter) Acquire() bufferBorrowed {
	return bufferBorrowed{project: a.pool.Buffer(0)}
}
func (*projectBufferAdapter) Write(borrowed *bufferBorrowed, data []byte) {
	if _, err := borrowed.project.Write(data); err != nil {
		panic(err)
	}
}
func (*projectBufferAdapter) Bytes(borrowed *bufferBorrowed) []byte {
	return borrowed.project.Bytes()
}
func (*projectBufferAdapter) Release(borrowed *bufferBorrowed) {
	borrowed.project.Release()
}

func bufferAdapters() []bufferAdapter {
	boundedConfig := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
	boundedConfig.MaxRetainedCapacity = 64 << 20
	return []bufferAdapter{
		newBytesBufferAdapter{},
		&syncBytesBufferAdapter{name: "sync.Pool/bytes.Buffer/Naive"},
		&syncBytesBufferAdapter{name: "sync.Pool/bytes.Buffer/Cutoff", maxRetainedCapacity: 1 << 20},
		&valyalaBufferAdapter{},
		&bpoolBufferAdapter{name: "bpool/BufferPool", pool: bpool.NewBufferPool(64)},
		&bpoolBufferAdapter{name: "bpool/SizedBufferPool", pool: bpool.NewSizedBufferPool(64, 4096)},
		&projectBufferAdapter{name: "Project/Fast/Buffer", pool: mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))},
		&projectBufferAdapter{name: "Project/Bounded/Buffer", pool: mustPool(boundedConfig)},
	}
}

func mustPool(config bytebufferpool.Config) *bytebufferpool.Pool {
	pool, err := bytebufferpool.New(config)
	if err != nil {
		panic(err)
	}
	return pool
}
