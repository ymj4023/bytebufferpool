package bytebufferpool_test

import (
	"errors"
	"sync"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestBoundedPoolEnforcesRetainedCapacity(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64},
		MaxPooledCapacity:   64,
		MaxRetainedCapacity: 64,
		StatsEnabled:        false,
	})
	if err != nil {
		t.Fatalf("New Bounded Pool: %v", err)
	}

	first := pool.Acquire(64)
	second := pool.Acquire(64)
	if got := first.Release(); got != bytebufferpool.Retained {
		t.Fatalf("first Release() = %v; want Retained", got)
	}
	if got := second.Release(); got != bytebufferpool.DroppedFull {
		t.Fatalf("second Release() = %v; want DroppedFull", got)
	}

	stats := pool.Stats()
	if !stats.RetainedAvailable {
		t.Fatal("Bounded Stats reported retained inventory unavailable")
	}
	if stats.RetainedBuffers != 1 || stats.RetainedCapacity != 64 {
		t.Fatalf("Bounded retained inventory = %d buffers/%d bytes; want 1/64", stats.RetainedBuffers, stats.RetainedCapacity)
	}

	reused := pool.Acquire(64)
	stats = pool.Stats()
	if stats.RetainedBuffers != 0 || stats.RetainedCapacity != 0 {
		t.Fatalf("inventory after Acquire = %d buffers/%d bytes; want 0/0", stats.RetainedBuffers, stats.RetainedCapacity)
	}
	if got := reused.Release(); got != bytebufferpool.Retained {
		t.Fatalf("reused Release() = %v; want Retained", got)
	}
}

func TestBoundedPoolRejectsMissingBudget(t *testing.T) {
	_, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Bounded,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
	})
	if !errors.Is(err, bytebufferpool.ErrInvalidConfig) {
		t.Fatalf("New Bounded Pool without budget error = %v; want ErrInvalidConfig", err)
	}
}

func TestBoundedPoolNeverExceedsBudgetUnderConcurrentRelease(t *testing.T) {
	const (
		capacity = 64
		limit    = 8 * capacity
		leases   = 128
	)
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{capacity},
		MaxPooledCapacity:   capacity,
		MaxRetainedCapacity: limit,
	})
	if err != nil {
		t.Fatalf("New Bounded Pool: %v", err)
	}

	acquired := make([]bytebufferpool.Lease, leases)
	for i := range acquired {
		acquired[i] = pool.Acquire(capacity)
	}

	var wait sync.WaitGroup
	for i := range acquired {
		wait.Add(1)
		go func(lease *bytebufferpool.Lease) {
			defer wait.Done()
			lease.Release()
		}(&acquired[i])
	}
	wait.Wait()

	stats := pool.Stats()
	if stats.RetainedCapacity > limit {
		t.Fatalf("RetainedCapacity = %d; exceeds budget %d", stats.RetainedCapacity, limit)
	}
	if stats.RetainedBuffers*capacity != stats.RetainedCapacity {
		t.Fatalf("retained inventory inconsistent: %d buffers/%d bytes", stats.RetainedBuffers, stats.RetainedCapacity)
	}
}
