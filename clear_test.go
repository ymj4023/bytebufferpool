package bytebufferpool_test

import (
	"sync"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestClearDropsStaleLeasesAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		config bytebufferpool.Config
	}{
		{name: "Fast", config: bytebufferpool.Config{
			Mode:              bytebufferpool.Fast,
			Classes:           []int{64},
			MaxPooledCapacity: 64,
		}},
		{name: "Bounded", config: bytebufferpool.Config{
			Mode:                bytebufferpool.Bounded,
			Classes:             []int{64},
			MaxPooledCapacity:   64,
			MaxRetainedCapacity: 128,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, err := bytebufferpool.New(test.config)
			if err != nil {
				t.Fatalf("New(): %v", err)
			}

			stale := pool.Acquire(64)
			idle := pool.Acquire(64)
			if got := idle.Release(); got != bytebufferpool.Retained {
				t.Fatalf("seed idle Release() = %v; want Retained", got)
			}

			pool.Clear()
			stale.Bytes()[0] = 0x2a
			if got := stale.Release(); got != bytebufferpool.DroppedStale {
				t.Fatalf("pre-Clear Lease Release() = %v; want DroppedStale", got)
			}

			stats := pool.Stats()
			if test.config.Mode == bytebufferpool.Bounded && (stats.RetainedStorageCount != 0 || stats.RetainedCapacity != 0) {
				t.Fatalf("inventory after Clear = %d storage objects/%d bytes; want 0/0", stats.RetainedStorageCount, stats.RetainedCapacity)
			}

			current := pool.Acquire(64)
			if got := current.Release(); got != bytebufferpool.Retained {
				t.Fatalf("current Generation Release() = %v; want Retained", got)
			}
		})
	}
}

func TestClearIsSafeWithConcurrentAcquireAndRelease(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64},
		MaxPooledCapacity:   64,
		MaxRetainedCapacity: 8 * 64,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	const workers = 16
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				lease := pool.Acquire(64)
				lease.Bytes()[0] = byte(j)
				lease.Release()
			}
		}()
	}
	for i := 0; i < 100; i++ {
		pool.Clear()
	}
	wait.Wait()

	stats := pool.Stats()
	if stats.RetainedCapacity > 8*64 {
		t.Fatalf("RetainedCapacity = %d; exceeds budget after concurrent Clear", stats.RetainedCapacity)
	}
}
