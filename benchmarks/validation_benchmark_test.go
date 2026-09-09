package benchmarks

import (
	"fmt"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

// BenchmarkValidationHistory separates hot address reuse from batches of distinct
// live oversize addresses. Batches include payload allocations and diagnostic
// filling; comparisons measure the whole validation option, not FIFO alone.
func BenchmarkValidationHistory(b *testing.B) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, limit := range []int{0, 1, 16384} {
			for _, batch := range []bool{false, true} {
				b.Run(fmt.Sprintf("mode=%d/limit=%d/batch=%t", mode, limit, batch), func(b *testing.B) {
					config := bytebufferpool.DefaultConfig(mode)
					config.Classes = []int{64}
					config.MaxPooledCapacity = 64
					config.ValidationEnabled = limit != 0
					config.MaxValidationTombstones = limit
					pool := mustPool(config)
					var buffers [128][]byte
					size, count := 64, 1
					want := bytebufferpool.Retained
					if batch {
						size, count, want = 65, len(buffers), bytebufferpool.DroppedOversize
					}
					b.ReportAllocs()
					b.SetBytes(int64(size * count))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						for j := 0; j < count; j++ {
							buffers[j] = pool.AcquireSlice(size)
							buffers[j][0] = byte(i)
						}
						for j := 0; j < count; j++ {
							if got := pool.ReleaseSlice(buffers[j]); got != want {
								b.Fatalf("ReleaseSlice = %v; want %v", got, want)
							}
							buffers[j] = nil
						}
					}
				})
			}
		}
	}
}
