package bytebufferpool_test

import (
	"errors"
	"reflect"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestFastPoolAcquiresDeterministicLease(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("construct default Fast Pool: %v", err)
	}

	lease := pool.Acquire(65)
	if got := lease.Len(); got != 65 {
		t.Fatalf("Lease.Len() = %d; want 65", got)
	}
	if got := lease.Cap(); got != 128 {
		t.Fatalf("Lease.Cap() = %d; want deterministic 128-byte Capacity Class", got)
	}

	lease.Bytes()[0] = 0x2a
	if got := lease.Bytes()[0]; got != 0x2a {
		t.Fatalf("Lease.Bytes()[0] = %#x; want %#x", got, byte(0x2a))
	}

	if got := lease.Release(); got != bytebufferpool.Retained {
		t.Fatalf("Lease.Release() = %v; want Retained", got)
	}
}

func TestPoolRejectsAmbiguousCapacityClasses(t *testing.T) {
	tests := []struct {
		name    string
		classes []int
		max     int
	}{
		{name: "non-positive", classes: []int{0, 64}, max: 64},
		{name: "duplicate", classes: []int{64, 64}, max: 64},
		{name: "unordered", classes: []int{128, 64}, max: 128},
		{name: "above pooling cutoff", classes: []int{64, 256}, max: 128},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := bytebufferpool.New(bytebufferpool.Config{
				Mode:              bytebufferpool.Fast,
				Classes:           test.classes,
				MaxPooledCapacity: test.max,
			})
			if !errors.Is(err, bytebufferpool.ErrInvalidConfig) {
				t.Fatalf("New() error = %v; want ErrInvalidConfig", err)
			}
		})
	}
}

func TestPoolCopiesCapacityClassesAtConstruction(t *testing.T) {
	classes := []int{64, 128}
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           classes,
		MaxPooledCapacity: 128,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	classes[1] = 65

	lease := pool.Acquire(65)
	defer lease.Release()
	if got := lease.Cap(); got != 128 {
		t.Fatalf("Lease.Cap() = %d after caller mutated Config.Classes; want immutable 128", got)
	}
}

func TestPowerOfTwoBuildsExplicitCapacityClasses(t *testing.T) {
	classes, err := bytebufferpool.PowerOfTwo(64, 512)
	if err != nil {
		t.Fatalf("PowerOfTwo(): %v", err)
	}
	if want := []int{64, 128, 256, 512}; !reflect.DeepEqual(classes, want) {
		t.Fatalf("PowerOfTwo() = %v; want %v", classes, want)
	}

	for _, bounds := range [][2]int{{0, 64}, {96, 512}, {64, 96}, {512, 64}} {
		_, err := bytebufferpool.PowerOfTwo(bounds[0], bounds[1])
		if !errors.Is(err, bytebufferpool.ErrInvalidConfig) {
			t.Errorf("PowerOfTwo(%d, %d) error = %v; want ErrInvalidConfig", bounds[0], bounds[1], err)
		}
	}
}

func TestPoolDistinguishesInvalidSizesAndUnpooledStorage(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64, 128},
		MaxPooledCapacity: 256,
		MaxAcquireSize:    512,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	for _, size := range []int{-1, 513} {
		if _, err := pool.TryAcquire(size); !errors.Is(err, bytebufferpool.ErrInvalidSize) {
			t.Errorf("TryAcquire(%d) error = %v; want ErrInvalidSize", size, err)
		}
	}

	t.Run("trusted invalid size panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("Acquire(-1) did not panic")
			}
		}()
		pool.Acquire(-1)
	})

	classGap := pool.Acquire(200)
	if got := classGap.Release(); got != bytebufferpool.DroppedInvalid {
		t.Fatalf("Release 200-byte class gap = %v; want DroppedInvalid", got)
	}

	oversize := pool.Acquire(300)
	if got := oversize.Release(); got != bytebufferpool.DroppedOversize {
		t.Fatalf("Release 300-byte oversize = %v; want DroppedOversize", got)
	}

	usable := pool.Acquire(64)
	defer usable.Release()
	if got := usable.Len(); got != 64 {
		t.Fatalf("Pool unusable after size errors: Lease.Len() = %d; want 64", got)
	}
}

func TestLeaseRejectsDuplicateReleaseAndPostReleaseUse(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	lease := pool.Acquire(64)

	if got := lease.Release(); got != bytebufferpool.Retained {
		t.Fatalf("first Release() = %v; want Retained", got)
	}
	if got := lease.Release(); got != bytebufferpool.RejectedDuplicate {
		t.Fatalf("second Release() = %v; want RejectedDuplicate", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Lease.Bytes() after Release did not panic")
		}
	}()
	lease.Bytes()
}

func TestLeaseCopyCannotReleaseBackingStorageTwice(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	t.Run("copy before first use", func(t *testing.T) {
		lease := pool.Acquire(64)
		copied := lease
		if got := lease.Release(); got != bytebufferpool.Retained {
			t.Fatalf("original Release() = %v; want Retained", got)
		}
		if got := copied.Release(); got != bytebufferpool.RejectedDuplicate {
			t.Fatalf("copied Release() = %v; want RejectedDuplicate", got)
		}
	})

	t.Run("copy after first use panics", func(t *testing.T) {
		lease := pool.Acquire(64)
		defer lease.Release()
		lease.Bytes()
		copied := lease

		defer func() {
			if recover() == nil {
				t.Fatal("using a copied Lease did not panic")
			}
		}()
		copied.Len()
	})
}

func TestPoolRejectsEveryInvalidConfigurationShape(t *testing.T) {
	tests := []struct {
		name   string
		config bytebufferpool.Config
	}{
		{name: "unsupported Mode", config: bytebufferpool.Config{Mode: bytebufferpool.Mode(99)}},
		{name: "negative cutoff", config: bytebufferpool.Config{Mode: bytebufferpool.Fast, MaxPooledCapacity: -1}},
		{name: "negative acquisition limit", config: bytebufferpool.Config{Mode: bytebufferpool.Fast, MaxAcquireSize: -1}},
		{name: "limit below cutoff", config: bytebufferpool.Config{Mode: bytebufferpool.Fast, Classes: []int{64}, MaxPooledCapacity: 64, MaxAcquireSize: 32}},
		{name: "Fast retained budget", config: bytebufferpool.Config{Mode: bytebufferpool.Fast, MaxRetainedCapacity: 1}},
		{name: "Bounded missing budget", config: bytebufferpool.Config{Mode: bytebufferpool.Bounded}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := bytebufferpool.New(test.config); !errors.Is(err, bytebufferpool.ErrInvalidConfig) {
				t.Fatalf("New() error = %v; want ErrInvalidConfig", err)
			}
		})
	}
}

func TestDefaultPoolRoutesEveryCapacityClassBoundary(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	for capacity := 64; capacity <= 1<<20; capacity <<= 1 {
		for _, size := range []int{capacity - 1, capacity} {
			lease := pool.Acquire(size)
			if got := lease.Cap(); got != capacity {
				t.Fatalf("Acquire(%d) cap = %d; want Capacity Class %d", size, got, capacity)
			}
			if got := lease.Release(); got != bytebufferpool.Retained {
				t.Fatalf("Release(%d) = %v; want Retained", size, got)
			}
		}
		if capacity < 1<<20 {
			lease := pool.Acquire(capacity + 1)
			if got := lease.Cap(); got != capacity<<1 {
				t.Fatalf("Acquire(%d) cap = %d; want next Capacity Class %d", capacity+1, got, capacity<<1)
			}
			lease.Release()
		}
	}

	oversize := pool.Acquire((1 << 20) + 1)
	if got := oversize.Cap(); got != (1<<20)+1 {
		t.Fatalf("oversize cap = %d; want exact %d", got, (1<<20)+1)
	}
	if got := oversize.Release(); got != bytebufferpool.DroppedOversize {
		t.Fatalf("oversize Release() = %v; want DroppedOversize", got)
	}
}
