package benchmarks

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

var benchmarkSink atomic.Uint64

func BenchmarkRawFixed(b *testing.B) {
	for _, size := range []int{64, 1024, 16 << 10, 64 << 10, 1 << 20} {
		for _, adapter := range rawAdapters() {
			b.Run(fmt.Sprintf("%d/%s", size, adapter.Name()), func(b *testing.B) {
				benchmarkRawSize(b, adapter, size)
			})
		}
	}
}

func BenchmarkRawBoundary(b *testing.B) {
	for _, size := range capacityBoundaries() {
		for _, adapter := range rawAdapters() {
			b.Run(fmt.Sprintf("%d/%s", size, adapter.Name()), func(b *testing.B) {
				benchmarkRawSize(b, adapter, size)
			})
		}
	}
}

func BenchmarkRawMixed(b *testing.B) {
	trace, average := mixedTrace()
	for _, adapter := range rawAdapters() {
		b.Run(adapter.Name(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(average))
			b.ResetTimer()
			var borrowed rawBorrowed
			for i := 0; i < b.N; i++ {
				size := trace[i%len(trace)]
				adapter.Acquire(size, &borrowed)
				touchRaw(borrowed.bytes)
				adapter.Release(&borrowed)
			}
		})
	}
}

func BenchmarkRawParallel(b *testing.B) {
	const size = 1024
	for _, adapter := range rawAdapters() {
		b.Run(adapter.Name(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(size)
			b.RunParallel(func(iterations *testing.PB) {
				var borrowed rawBorrowed
				for iterations.Next() {
					adapter.Acquire(size, &borrowed)
					touchRaw(borrowed.bytes)
					adapter.Release(&borrowed)
				}
			})
		})
	}
}

func BenchmarkBufferFixed(b *testing.B) {
	for _, size := range []int{1024, 16 << 10, 1 << 20} {
		chunks := appendChunks(size)
		for _, adapter := range bufferAdapters() {
			b.Run(fmt.Sprintf("%d/%s", size, adapter.Name()), func(b *testing.B) {
				benchmarkBufferChunks(b, adapter, chunks, size)
			})
		}
	}
}

func BenchmarkBufferParallel(b *testing.B) {
	const size = 16 << 10
	chunks := appendChunks(size)
	for _, adapter := range bufferAdapters() {
		b.Run(adapter.Name(), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(size)
			b.RunParallel(func(iterations *testing.PB) {
				for iterations.Next() {
					borrowed := adapter.Acquire()
					for _, chunk := range chunks {
						adapter.Write(&borrowed, chunk)
					}
					touchRaw(adapter.Bytes(&borrowed))
					adapter.Release(&borrowed)
				}
			})
		})
	}
}

func BenchmarkRawLifecycle(b *testing.B) {
	const size = 1024
	b.Run("Cold/Project/Fast/Raw", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(size)
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			pool := mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
			b.StartTimer()
			buffer := pool.AcquireSlice(size)
			touchRaw(buffer)
			pool.ReleaseSlice(buffer)
		}
	})

	b.Run("Warm/Project/Fast/Raw", func(b *testing.B) {
		pool := mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
		seed := pool.AcquireSlice(size)
		pool.ReleaseSlice(seed)
		b.ReportAllocs()
		b.SetBytes(size)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buffer := pool.AcquireSlice(size)
			touchRaw(buffer)
			pool.ReleaseSlice(buffer)
		}
	})

	b.Run("PostGC/Project/Fast/Raw", func(b *testing.B) {
		pool := mustPool(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
		b.ReportAllocs()
		b.SetBytes(size)
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			seed := pool.AcquireSlice(size)
			pool.ReleaseSlice(seed)
			runtime.GC()
			b.StartTimer()
			buffer := pool.AcquireSlice(size)
			touchRaw(buffer)
			pool.ReleaseSlice(buffer)
		}
	})
}

func BenchmarkBoundedBudgetExhaustion(b *testing.B) {
	const capacity = 64
	b.ReportAllocs()
	b.SetBytes(2 * capacity)
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pool := mustPool(bytebufferpool.Config{
			Mode:                bytebufferpool.Bounded,
			Classes:             []int{capacity},
			MaxPooledCapacity:   capacity,
			MaxRetainedCapacity: capacity,
		})
		first := pool.Acquire(capacity)
		second := pool.Acquire(capacity)
		b.StartTimer()
		firstStatus := first.Release()
		secondStatus := second.Release()
		b.StopTimer()
		if firstStatus != bytebufferpool.Retained || secondStatus != bytebufferpool.DroppedFull {
			b.Fatalf("Release statuses = %v/%v; want Retained/DroppedFull", firstStatus, secondStatus)
		}
	}
}

func benchmarkRawSize(b *testing.B, adapter rawAdapter, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ResetTimer()
	var borrowed rawBorrowed
	for i := 0; i < b.N; i++ {
		adapter.Acquire(size, &borrowed)
		touchRaw(borrowed.bytes)
		adapter.Release(&borrowed)
	}
}

func benchmarkBufferChunks(b *testing.B, adapter bufferAdapter, chunks [][]byte, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		borrowed := adapter.Acquire()
		for _, chunk := range chunks {
			adapter.Write(&borrowed, chunk)
		}
		touchRaw(adapter.Bytes(&borrowed))
		adapter.Release(&borrowed)
	}
}

func touchRaw(buffer []byte) {
	if len(buffer) == 0 {
		return
	}
	buffer[0]++
	buffer[len(buffer)-1]++
	benchmarkSink.Add(uint64(buffer[0] ^ buffer[len(buffer)-1]))
}

func capacityBoundaries() []int {
	boundaries := make([]int, 0, 45)
	for capacity := 64; capacity <= 1<<20; capacity <<= 1 {
		boundaries = append(boundaries, capacity-1, capacity, capacity+1)
	}
	return boundaries
}

func mixedTrace() ([]int, int) {
	trace := make([]int, 1000)
	total := 0
	for i := range trace {
		switch {
		case i < 700:
			trace[i] = 512
		case i < 900:
			trace[i] = 4 << 10
		case i < 990:
			trace[i] = 64 << 10
		default:
			trace[i] = 1 << 20
		}
		total += trace[i]
	}
	return trace, total / len(trace)
}

func appendChunks(size int) [][]byte {
	const chunkSize = 128
	chunks := make([][]byte, 0, (size+chunkSize-1)/chunkSize)
	remaining := size
	for remaining > 0 {
		length := min(chunkSize, remaining)
		chunk := make([]byte, length)
		for i := range chunk {
			chunk[i] = byte(i)
		}
		chunks = append(chunks, chunk)
		remaining -= length
	}
	return chunks
}
