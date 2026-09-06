package bytebufferpool_test

import (
	"errors"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestRawSliceSharesCapacityContractAcrossBackends(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		config := bytebufferpool.Config{
			Mode:              mode,
			Classes:           []int{64, 128},
			MaxPooledCapacity: 128,
		}
		if mode == bytebufferpool.Bounded {
			config.MaxRetainedCapacity = 256
		}

		pool, err := bytebufferpool.New(config)
		if err != nil {
			t.Fatalf("New(%v): %v", mode, err)
		}
		buffer := pool.AcquireSlice(65)
		if len(buffer) != 65 || cap(buffer) != 128 {
			t.Fatalf("AcquireSlice(65) = len %d/cap %d; want 65/128", len(buffer), cap(buffer))
		}
		buffer[0] = 0x2a
		if got := pool.ReleaseSlice(buffer); got != bytebufferpool.Retained {
			t.Fatalf("ReleaseSlice() = %v; want Retained", got)
		}

		if got := pool.AcquireSlice(0); got != nil {
			t.Fatalf("AcquireSlice(0) = %#v; want nil", got)
		}
		if got := pool.ReleaseSlice(nil); got != bytebufferpool.IgnoredNil {
			t.Fatalf("ReleaseSlice(nil) = %v; want IgnoredNil", got)
		}
	}
}

func TestRawSliceRejectsInvalidSizeAndReplacementBackingStorage(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		MaxAcquireSize:    128,
		ValidationEnabled: true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	for _, size := range []int{-1, 129} {
		if _, err := pool.TryAcquireSlice(size); !errors.Is(err, bytebufferpool.ErrInvalidSize) {
			t.Errorf("TryAcquireSlice(%d) error = %v; want ErrInvalidSize", size, err)
		}
	}

	original := pool.AcquireSlice(64)
	replacement := append(original, 0x7f)
	if got := pool.ReleaseSlice(replacement); got != bytebufferpool.RejectedForeign {
		t.Fatalf("ReleaseSlice(replacement) = %v; want RejectedForeign", got)
	}
	if got := pool.ReleaseSlice(original); got != bytebufferpool.Retained {
		t.Fatalf("ReleaseSlice(original) = %v; want Retained", got)
	}
}

func TestRawSliceReleaseAfterClearUsesCurrentGeneration(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	buffer := pool.AcquireSlice(64)
	pool.Clear()
	if got := pool.ReleaseSlice(buffer); got != bytebufferpool.Retained {
		t.Fatalf("pre-Clear Raw Slice Release() = %v; want current-generation Retained", got)
	}
}

func TestRawSliceEnhancedValidationRejectsOwnershipErrors(t *testing.T) {
	newPool := func(t *testing.T) *bytebufferpool.Pool {
		t.Helper()
		pool, err := bytebufferpool.New(bytebufferpool.Config{
			Mode:              bytebufferpool.Fast,
			Classes:           []int{64, 128},
			MaxPooledCapacity: 128,
			ValidationEnabled: true,
		})
		if err != nil {
			t.Fatalf("New(): %v", err)
		}
		return pool
	}

	owner := newPool(t)
	other := newPool(t)
	buffer := owner.AcquireSlice(64)
	buffer[0] = 0x44

	if got := other.ReleaseSlice(buffer); got != bytebufferpool.RejectedForeign {
		t.Fatalf("cross-Pool ReleaseSlice() = %v; want RejectedForeign", got)
	}
	if buffer[0] != 0x44 {
		t.Fatalf("cross-Pool rejection modified buffer[0] to %#x", buffer[0])
	}

	if got := owner.ReleaseSlice(buffer); got != bytebufferpool.Retained {
		t.Fatalf("owner ReleaseSlice() = %v; want Retained", got)
	}
	buffer[0] = 0x55 // Deliberate stale alias probes duplicate-rejection mutation.
	if got := owner.ReleaseSlice(buffer); got != bytebufferpool.RejectedDuplicate {
		t.Fatalf("duplicate ReleaseSlice() = %v; want RejectedDuplicate", got)
	}
	if buffer[0] != 0x55 {
		t.Fatalf("duplicate rejection modified buffer[0] to %#x", buffer[0])
	}

	foreign := make([]byte, 64)
	foreign[0] = 0x66
	if got := owner.ReleaseSlice(foreign); got != bytebufferpool.RejectedForeign {
		t.Fatalf("foreign ReleaseSlice() = %v; want RejectedForeign", got)
	}
	if foreign[0] != 0x66 {
		t.Fatalf("foreign rejection modified buffer[0] to %#x", foreign[0])
	}
}

func TestRawSliceEnhancedValidationRejectsChangedCapacity(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64, 128},
		MaxPooledCapacity: 128,
		ValidationEnabled: true,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	buffer := pool.AcquireSlice(65)
	changedCapacity := buffer[:64:64]
	if got := pool.ReleaseSlice(changedCapacity); got != bytebufferpool.DroppedInvalid {
		t.Fatalf("ReleaseSlice() after capacity change = %v; want DroppedInvalid", got)
	}
}

func TestRawSliceValidationEvictsOldestInactiveTombstone(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                    bytebufferpool.Fast,
		Classes:                 []int{64},
		MaxPooledCapacity:       64,
		ValidationEnabled:       true,
		MaxValidationTombstones: 2,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	raw := [][]byte{
		pool.AcquireSlice(64),
		pool.AcquireSlice(64),
		pool.AcquireSlice(64),
	}
	for i := range raw {
		if status := pool.ReleaseSlice(raw[i]); status != bytebufferpool.Retained {
			t.Fatalf("ReleaseSlice(%d) = %v; want Retained", i, status)
		}
		raw[i][0] = byte(0x40 + i)
	}

	stats := pool.Stats()
	if stats.ActiveRawSlices != 0 || stats.ValidationTombstones != 2 {
		t.Fatalf("Validation Inventory = active %d/tombstones %d; want 0/2", stats.ActiveRawSlices, stats.ValidationTombstones)
	}
	wantStatuses := []bytebufferpool.ReleaseStatus{
		bytebufferpool.RejectedForeign,
		bytebufferpool.RejectedDuplicate,
		bytebufferpool.RejectedDuplicate,
	}
	for i, want := range wantStatuses {
		if status := pool.ReleaseSlice(raw[i]); status != want {
			t.Fatalf("second ReleaseSlice(%d) = %v; want %v", i, status, want)
		}
		if raw[i][0] != byte(0x40+i) {
			t.Fatalf("rejected ReleaseSlice(%d) modified byte to %#x", i, raw[i][0])
		}
	}
}

func TestRawSliceValidationClearPreservesActiveOwnership(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                    bytebufferpool.Bounded,
		Classes:                 []int{64},
		MaxPooledCapacity:       64,
		MaxRetainedCapacity:     128,
		ValidationEnabled:       true,
		MaxValidationTombstones: 2,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	active := pool.AcquireSlice(64)
	tombstone := pool.AcquireSlice(64)
	if status := pool.ReleaseSlice(tombstone); status != bytebufferpool.Retained {
		t.Fatalf("tombstone ReleaseSlice() = %v; want Retained", status)
	}
	stats := pool.Stats()
	if stats.ActiveRawSlices != 1 || stats.ValidationTombstones != 1 {
		t.Fatalf("Validation Inventory before Clear = active %d/tombstones %d; want 1/1", stats.ActiveRawSlices, stats.ValidationTombstones)
	}

	tombstone[0] = 0x5a
	pool.Clear()
	stats = pool.Stats()
	if stats.ActiveRawSlices != 1 || stats.ValidationTombstones != 0 {
		t.Fatalf("Validation Inventory after Clear = active %d/tombstones %d; want 1/0", stats.ActiveRawSlices, stats.ValidationTombstones)
	}
	if status := pool.ReleaseSlice(tombstone); status != bytebufferpool.RejectedForeign {
		t.Fatalf("cleared tombstone ReleaseSlice() = %v; want RejectedForeign", status)
	}
	if tombstone[0] != 0x5a {
		t.Fatalf("cleared tombstone rejection modified byte to %#x", tombstone[0])
	}
	if status := pool.ReleaseSlice(active); status != bytebufferpool.Retained {
		t.Fatalf("active pre-Clear ReleaseSlice() = %v; want Retained", status)
	}
	stats = pool.Stats()
	if stats.ActiveRawSlices != 0 || stats.ValidationTombstones != 1 {
		t.Fatalf("Validation Inventory after active Release = active %d/tombstones %d; want 0/1", stats.ActiveRawSlices, stats.ValidationTombstones)
	}
}

func TestRawSliceValidationKeepsReusedAddressActiveAboveTombstoneLimit(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                    bytebufferpool.Bounded,
		Classes:                 []int{64},
		MaxPooledCapacity:       64,
		MaxRetainedCapacity:     64,
		ValidationEnabled:       true,
		MaxValidationTombstones: 1,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	original := pool.AcquireSlice(64)
	if status := pool.ReleaseSlice(original); status != bytebufferpool.Retained {
		t.Fatalf("original ReleaseSlice() = %v; want Retained", status)
	}
	reused := pool.AcquireSlice(64)
	if &original[0] != &reused[0] {
		t.Fatal("Bounded LIFO did not return the retained Backing Storage needed for the address-reuse scenario")
	}
	firstOversize := pool.AcquireSlice(65)
	secondOversize := pool.AcquireSlice(65)
	stats := pool.Stats()
	if stats.ActiveRawSlices != 3 || stats.ValidationTombstones != 0 {
		t.Fatalf("Validation Inventory above limit = active %d/tombstones %d; want 3/0", stats.ActiveRawSlices, stats.ValidationTombstones)
	}

	if status := pool.ReleaseSlice(firstOversize); status != bytebufferpool.DroppedOversize {
		t.Fatalf("first oversize ReleaseSlice() = %v; want DroppedOversize", status)
	}
	if status := pool.ReleaseSlice(secondOversize); status != bytebufferpool.DroppedOversize {
		t.Fatalf("second oversize ReleaseSlice() = %v; want DroppedOversize", status)
	}
	stats = pool.Stats()
	if stats.ActiveRawSlices != 1 || stats.ValidationTombstones != 1 {
		t.Fatalf("Validation Inventory after FIFO eviction = active %d/tombstones %d; want 1/1", stats.ActiveRawSlices, stats.ValidationTombstones)
	}
	if status := pool.ReleaseSlice(reused); status != bytebufferpool.Retained {
		t.Fatalf("reused active ReleaseSlice() = %v; want Retained", status)
	}
}
