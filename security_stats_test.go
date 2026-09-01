package bytebufferpool_test

import (
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestZeroOnReleaseClearsFullCapacityAcrossAPIs(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64, 128},
		MaxPooledCapacity:   128,
		MaxRetainedCapacity: 512,
		ZeroOnRelease:       true,
		ValidationEnabled:   true,
		StatsEnabled:        true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	lease := pool.Acquire(65)
	leaseAlias := lease.Bytes()[:lease.Cap()]
	fillBytes(leaseAlias, 0x11)
	if got := lease.Release(); got != bytebufferpool.Retained {
		t.Fatalf("Lease.Release() = %v; want Retained", got)
	}
	assertAllZero(t, "Lease", leaseAlias)

	raw := pool.AcquireSlice(65)
	rawAlias := raw[:cap(raw)]
	fillBytes(rawAlias, 0x22)
	if got := pool.ReleaseSlice(raw); got != bytebufferpool.Retained {
		t.Fatalf("ReleaseSlice() = %v; want Retained", got)
	}
	assertAllZero(t, "Raw Slice", rawAlias)

	buffer := pool.Buffer(65)
	buffer.WriteString("secret")
	bufferAlias := buffer.Bytes()[:buffer.Cap()]
	fillBytes(bufferAlias, 0x33)
	if got := buffer.Release(); got != bytebufferpool.Retained {
		t.Fatalf("Buffer.Release() = %v; want Retained", got)
	}
	assertAllZero(t, "Buffer", bufferAlias)

	stats := pool.Stats()
	if !stats.CountersAvailable {
		t.Fatal("StatsEnabled Pool reported counters unavailable")
	}
	if stats.ZeroedBytes != 3*128 {
		t.Fatalf("ZeroedBytes = %d; want %d", stats.ZeroedBytes, 3*128)
	}
}

func TestZeroOnReleaseDoesNotModifyRejectedStorage(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		ZeroOnRelease:     true,
		ValidationEnabled: true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	foreign := make([]byte, 64)
	fillBytes(foreign, 0x44)
	if got := pool.ReleaseSlice(foreign); got != bytebufferpool.RejectedForeign {
		t.Fatalf("foreign ReleaseSlice() = %v; want RejectedForeign", got)
	}
	for i, value := range foreign {
		if value != 0x44 {
			t.Fatalf("foreign[%d] = %#x; rejection modified storage", i, value)
		}
	}

	raw := pool.AcquireSlice(64)
	if got := pool.ReleaseSlice(raw); got != bytebufferpool.Retained {
		t.Fatalf("first ReleaseSlice() = %v; want Retained", got)
	}
	raw[0] = 0x55
	if got := pool.ReleaseSlice(raw); got != bytebufferpool.RejectedDuplicate {
		t.Fatalf("duplicate ReleaseSlice() = %v; want RejectedDuplicate", got)
	}
	if raw[0] != 0x55 {
		t.Fatalf("duplicate rejection changed raw[0] to %#x", raw[0])
	}
}

func TestZeroOnReleaseClearsOriginalCapacityAfterRawReslice(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64, 128},
		MaxPooledCapacity: 128,
		ZeroOnRelease:     true,
		ValidationEnabled: true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	original := pool.AcquireSlice(65)
	full := original[:cap(original)]
	fillBytes(full, 0x77)
	shortCapacity := original[:64:64]
	if got := pool.ReleaseSlice(shortCapacity); got != bytebufferpool.DroppedInvalid {
		t.Fatalf("ReleaseSlice(short capacity) = %v; want DroppedInvalid", got)
	}
	assertAllZero(t, "resliced Raw Slice", full)
}

func TestOptionalStatsReportDeterministicBoundedOperations(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64},
		MaxPooledCapacity:   64,
		MaxRetainedCapacity: 64,
		StatsEnabled:        true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	first := pool.Acquire(64) // miss
	second := pool.Acquire(64)
	first.Release()            // retained
	reused := pool.Acquire(64) // hit
	second.Release()           // retained
	if got := reused.Release(); got != bytebufferpool.DroppedFull {
		t.Fatalf("Release over budget = %v; want DroppedFull", got)
	}

	stats := pool.Stats()
	if !stats.CountersAvailable {
		t.Fatal("StatsEnabled Pool reported counters unavailable")
	}
	if stats.Acquires != 3 || stats.Hits != 1 || stats.Misses != 2 {
		t.Fatalf("acquire counters = %d total/%d hits/%d misses; want 3/1/2", stats.Acquires, stats.Hits, stats.Misses)
	}
	if stats.Releases != 3 || stats.Retained != 2 || stats.DroppedFull != 1 {
		t.Fatalf("release counters = %d releases/%d retained/%d full; want 3/2/1", stats.Releases, stats.Retained, stats.DroppedFull)
	}
	if len(stats.Classes) != 1 {
		t.Fatalf("len(Stats.Classes) = %d; want 1", len(stats.Classes))
	}
	class := stats.Classes[0]
	if class.Capacity != 64 || class.Hits != 1 || class.Misses != 2 || class.Retained != 2 || class.Dropped != 1 {
		t.Fatalf("ClassStats = %+v; want capacity 64, hit/miss 1/2, retained/dropped 2/1", class)
	}
}

func TestOptionalStatsStayDisabledWithoutLosingBoundedInventory(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64},
		MaxPooledCapacity:   64,
		MaxRetainedCapacity: 64,
		StatsEnabled:        false,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	lease := pool.Acquire(64)
	lease.Release()

	stats := pool.Stats()
	if stats.CountersAvailable || stats.Acquires != 0 || stats.Releases != 0 || len(stats.Classes) != 0 {
		t.Fatalf("disabled optional counters leaked data: %+v", stats)
	}
	if !stats.RetainedAvailable || stats.RetainedStorageCount != 1 || stats.RetainedCapacity != 64 {
		t.Fatalf("mandatory Bounded inventory lost when counters disabled: %+v", stats)
	}
}

func TestRawSliceRecordsZeroOversizeAndInvalidOperations(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		MaxAcquireSize:    128,
		StatsEnabled:      true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	if got := pool.AcquireSlice(0); got != nil {
		t.Fatalf("AcquireSlice(0) = %#v; want nil", got)
	}
	oversize := pool.AcquireSlice(65)
	if got := pool.ReleaseSlice(oversize); got != bytebufferpool.DroppedOversize {
		t.Fatalf("oversize ReleaseSlice() = %v; want DroppedOversize", got)
	}
	if got := pool.ReleaseSlice(make([]byte, 32)); got != bytebufferpool.DroppedInvalid {
		t.Fatalf("invalid ReleaseSlice() = %v; want DroppedInvalid", got)
	}

	stats := pool.Stats()
	if stats.Acquires != 2 || stats.Misses != 2 {
		t.Fatalf("Raw acquire counters = %d/%d misses; want 2/2", stats.Acquires, stats.Misses)
	}
	if stats.Releases != 2 || stats.DroppedOversize != 1 || stats.DroppedInvalid != 1 {
		t.Fatalf("Raw release counters = %d releases, oversize %d, invalid %d; want 2/1/1", stats.Releases, stats.DroppedOversize, stats.DroppedInvalid)
	}
}

func fillBytes(buffer []byte, value byte) {
	for i := range buffer {
		buffer[i] = value
	}
}

func assertAllZero(t *testing.T, name string, buffer []byte) {
	t.Helper()
	for i, value := range buffer {
		if value != 0 {
			t.Fatalf("%s capacity byte %d = %#x; want zero", name, i, value)
		}
	}
}
