// validationmem measures one Pool in one process. Invoke it repeatedly from a
// parent shell to keep allocator state independent across samples.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

type sample struct {
	Phase      string `json:"phase"`
	Active     int64  `json:"active"`
	Tombstones int64  `json:"tombstones"`
	Limit      int64  `json:"limit"`
	HeapAlloc  uint64 `json:"heap_alloc"`
	HeapInuse  uint64 `json:"heap_inuse"`
}

func main() {
	peak := flag.Int("peak", 65536, "simultaneously active Raw Slices")
	limit := flag.Int("limit", 0, "inactive history limit; zero selects default")
	validation := flag.Bool("validation", true, "enable enhanced validation")
	flag.Parse()
	if *peak <= 0 {
		fmt.Fprintln(os.Stderr, "peak must be positive")
		os.Exit(2)
	}
	runtime.GOMAXPROCS(1)
	debug.SetGCPercent(-1)
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Classes: []int{64}, MaxPooledCapacity: 64,
		ValidationEnabled: *validation, MaxValidationTombstones: *limit,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	samples := []sample{observe(pool, "baseline")}
	raw := acquire(pool, *peak)
	samples = append(samples, observe(pool, "active_peak"))
	release(pool, raw)
	raw = nil
	samples = append(samples, observe(pool, "released_peak"))
	for i := 0; i < 1024; i++ {
		release(pool, acquire(pool, 128))
	}
	samples = append(samples, observe(pool, "after_churn"))
	pool.Clear()
	samples = append(samples, observe(pool, "after_clear"))
	result := struct {
		Go         string   `json:"go"`
		OS         string   `json:"os"`
		Arch       string   `json:"arch"`
		Peak       int      `json:"peak"`
		Validation bool     `json:"validation"`
		Samples    []sample `json:"samples"`
	}{runtime.Version(), runtime.GOOS, runtime.GOARCH, *peak, *validation, samples}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runtime.KeepAlive(pool)
}

func acquire(pool *bytebufferpool.Pool, count int) [][]byte {
	raw := make([][]byte, count)
	for i := range raw {
		raw[i] = pool.AcquireSlice(65) // Oversize: do not measure retained payload.
	}
	return raw
}

func release(pool *bytebufferpool.Pool, raw [][]byte) {
	for _, buffer := range raw {
		if status := pool.ReleaseSlice(buffer); status != bytebufferpool.DroppedOversize {
			panic(fmt.Sprintf("ReleaseSlice = %v; want DroppedOversize", status))
		}
	}
}

func observe(pool *bytebufferpool.Pool, phase string) sample {
	runtime.GC()
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	stats := pool.Stats()
	return sample{phase, stats.ActiveRawSlices, stats.ValidationTombstones, stats.MaxValidationTombstones, memory.HeapAlloc, memory.HeapInuse}
}
