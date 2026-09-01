package bytebufferpool_test

import (
	"errors"
	"sync"
	"sync/atomic"
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
	if stats.RetainedStorageCount != 1 || stats.RetainedCapacity != 64 {
		t.Fatalf("Bounded retained inventory = %d storage objects/%d bytes; want 1/64", stats.RetainedStorageCount, stats.RetainedCapacity)
	}

	reused := pool.Acquire(64)
	stats = pool.Stats()
	if stats.RetainedStorageCount != 0 || stats.RetainedCapacity != 0 {
		t.Fatalf("inventory after Acquire = %d storage objects/%d bytes; want 0/0", stats.RetainedStorageCount, stats.RetainedCapacity)
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
	if stats.RetainedStorageCount*capacity != stats.RetainedCapacity {
		t.Fatalf("retained inventory inconsistent: %d storage objects/%d bytes", stats.RetainedStorageCount, stats.RetainedCapacity)
	}
}

func TestBoundedPoolReportsExactInventoryDuringConcurrentReuse(t *testing.T) {
	const (
		capacity   = 64
		workers    = 8
		iterations = 10_000
	)
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{capacity},
		MaxPooledCapacity:   capacity,
		MaxRetainedCapacity: workers * capacity,
	})
	if err != nil {
		t.Fatalf("New Bounded Pool: %v", err)
	}

	var unexpectedReleases atomic.Int64
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				lease := pool.Acquire(capacity)
				if lease.Release() != bytebufferpool.Retained {
					unexpectedReleases.Add(1)
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()

	for {
		select {
		case <-done:
			if got := unexpectedReleases.Load(); got != 0 {
				t.Fatalf("unexpected release statuses = %d; want 0", got)
			}
			return
		default:
			stats := pool.Stats()
			if stats.RetainedStorageCount < 0 || stats.RetainedStorageCount > workers {
				t.Fatalf("RetainedStorageCount = %d; want within [0,%d]", stats.RetainedStorageCount, workers)
			}
			if stats.RetainedCapacity != stats.RetainedStorageCount*capacity {
				t.Fatalf("inventory snapshot = %d storage objects/%d bytes; want exact capacity", stats.RetainedStorageCount, stats.RetainedCapacity)
			}
		}
	}
}
