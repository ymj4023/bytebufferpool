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
			config.MaxRetainedBytes = 256
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
