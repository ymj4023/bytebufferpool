package bytebufferpool_test

import (
	"fmt"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestClassGapReleaseIsNormalDisposalAcrossAPIs(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, zero := range []bool{false, true} {
			for _, validation := range []bool{false, true} {
				t.Run(fmt.Sprintf("mode=%d/zero=%t/validation=%t", mode, zero, validation), func(t *testing.T) {
					config := bytebufferpool.DefaultConfig(mode)
					config.Classes = []int{64}
					config.MaxPooledCapacity = 128
					config.ZeroOnRelease = zero
					config.ValidationEnabled = validation
					pool, err := bytebufferpool.New(config)
					if err != nil {
						t.Fatal(err)
					}
					for _, size := range []int{65, 128, 129} {
						want := bytebufferpool.DroppedUnpooled
						if size == 129 {
							want = bytebufferpool.DroppedOversize
						}
						lease := pool.Acquire(size)
						alias := lease.Bytes()
						fillBytes(alias, 0x41)
						if got := lease.Release(); got != want {
							t.Fatalf("Lease(%d) = %v; want %v", size, got, want)
						}
						if zero {
							assertAllZero(t, "unpooled Lease", alias)
						}
						if got := lease.Release(); got != bytebufferpool.RejectedDuplicate {
							t.Fatalf("duplicate Lease = %v", got)
						}
						raw := pool.AcquireSlice(size)
						fillBytes(raw, 0x42)
						if got := pool.ReleaseSlice(raw); got != want {
							t.Fatalf("Raw(%d) = %v; want %v", size, got, want)
						}
						if zero {
							assertAllZero(t, "unpooled Raw Slice", raw)
						}
						if validation && pool.ReleaseSlice(raw) != bytebufferpool.RejectedDuplicate {
							t.Fatal("unpooled raw duplicate was not rejected")
						}
					}
				})
			}
		}
	}
}

func TestReleaseStatusPreservesPublishedNumbersAndNames(t *testing.T) {
	tests := []struct {
		status bytebufferpool.ReleaseStatus
		number uint8
		name   string
	}{
		{bytebufferpool.Retained, 0, "Retained"},
		{bytebufferpool.DroppedFull, 1, "DroppedFull"},
		{bytebufferpool.DroppedOversize, 2, "DroppedOversize"},
		{bytebufferpool.DroppedInvalid, 3, "DroppedInvalid"},
		{bytebufferpool.DroppedStale, 4, "DroppedStale"},
		{bytebufferpool.RejectedForeign, 5, "RejectedForeign"},
		{bytebufferpool.RejectedDuplicate, 6, "RejectedDuplicate"},
		{bytebufferpool.IgnoredNil, 7, "IgnoredNil"},
		{bytebufferpool.DroppedUnpooled, 8, "DroppedUnpooled"},
		{bytebufferpool.ReleaseStatus(255), 255, "ReleaseStatus(255)"},
	}
	for _, test := range tests {
		if uint8(test.status) != test.number || test.status.String() != test.name {
			t.Errorf("status %d/%s; want %d/%s", test.status, test.status, test.number, test.name)
		}
	}
}

func TestDroppedUnpooledCounterRemainsOptional(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
		config.Classes = []int{64}
		config.MaxPooledCapacity = 128
		config.StatsEnabled = enabled
		pool, err := bytebufferpool.New(config)
		if err != nil {
			t.Fatal(err)
		}
		lease := pool.Acquire(100)
		lease.Release()
		pool.ReleaseSlice(pool.AcquireSlice(100))
		stats := pool.Stats()
		want := uint64(0)
		if enabled {
			want = 2
		}
		if stats.DroppedUnpooled != want || stats.DroppedInvalid != 0 || stats.Releases != want || stats.CountersAvailable != enabled {
			t.Fatalf("counters enabled=%t: %+v", enabled, stats)
		}
		if !stats.RetainedAvailable || stats.RetainedCapacity != 0 || stats.RetainedStorageCount != 0 {
			t.Fatalf("unpooled storage entered retained inventory: %+v", stats)
		}
	}
}

func TestClassGapDoesNotOverrideOwnershipFailures(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, zero := range []bool{false, true} {
			config := bytebufferpool.DefaultConfig(mode)
			config.Classes = []int{64}
			config.MaxPooledCapacity = 128
			config.ValidationEnabled = true
			config.ZeroOnRelease = zero
			pool, err := bytebufferpool.New(config)
			if err != nil {
				t.Fatal(err)
			}
			raw := pool.AcquireSlice(100)
			if got := pool.ReleaseSlice(raw[:90:90]); got != bytebufferpool.DroppedInvalid {
				t.Fatalf("changed-capacity class gap = %v; want DroppedInvalid", got)
			}
			raw[0] = 0x43
			if got := pool.ReleaseSlice(raw); got != bytebufferpool.RejectedDuplicate || raw[0] != 0x43 {
				t.Fatalf("duplicate changed-capacity release = %v, byte %#x", got, raw[0])
			}
			foreign := make([]byte, 100)
			foreign[0] = 0x44
			if got := pool.ReleaseSlice(foreign); got != bytebufferpool.RejectedForeign || foreign[0] != 0x44 {
				t.Fatalf("foreign class gap = %v, byte %#x", got, foreign[0])
			}
			stale := pool.Acquire(100)
			pool.Clear()
			if got := stale.Release(); got != bytebufferpool.DroppedStale {
				t.Fatalf("stale class gap = %v; want DroppedStale", got)
			}
		}
	}
}

func TestRawClassGapWithoutValidationCannotProveProvenance(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatal(err)
	}
	// A foreign non-class allocation is indistinguishable without validation.
	if got := pool.ReleaseSlice(make([]byte, 32)); got != bytebufferpool.DroppedUnpooled {
		t.Fatalf("unvalidated non-class release = %v; want DroppedUnpooled", got)
	}
}
