package bytebufferpool_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestValidationConfigurationAndInventoryAvailability(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, counters := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				for _, limit := range []int{-1, 0, 3} {
					t.Run(fmt.Sprintf("mode=%d/counters=%t/validation=%t/limit=%d", mode, counters, enabled, limit), func(t *testing.T) {
						config := bytebufferpool.DefaultConfig(mode)
						config.ValidationEnabled = enabled
						config.StatsEnabled = counters
						config.MaxValidationTombstones = limit
						pool, err := bytebufferpool.New(config)
						if limit < 0 || (!enabled && limit != 0) {
							if !errors.Is(err, bytebufferpool.ErrInvalidConfig) {
								t.Fatalf("New() error = %v; want ErrInvalidConfig", err)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						raw := pool.AcquireSlice(64)
						stats := pool.Stats()
						wantLimit, wantActive := int64(0), int64(0)
						if enabled {
							wantActive = 1
							wantLimit = 16_384
							if limit > 0 {
								wantLimit = int64(limit)
							}
						}
						if stats.ValidationAvailable != enabled || stats.CountersAvailable != counters || stats.ActiveRawSlices != wantActive || stats.MaxValidationTombstones != wantLimit || stats.ValidationTombstones != 0 {
							t.Fatalf("availability or active inventory mismatch: %+v", stats)
						}
						pool.ReleaseSlice(raw)
					})
				}
			}
		}
	}
}

func TestValidationDuplicateDoesNotRefreshFIFOHistory(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprintf("zero=%t", zero), func(t *testing.T) {
			pool, err := bytebufferpool.New(bytebufferpool.Config{
				Classes: []int{64}, MaxPooledCapacity: 64,
				ValidationEnabled: true, MaxValidationTombstones: 2, ZeroOnRelease: zero,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, second, third := pool.AcquireSlice(65), pool.AcquireSlice(65), pool.AcquireSlice(65)
			pool.ReleaseSlice(first)
			pool.ReleaseSlice(second)
			first[0] = 0x71 // Deliberate alias misuse tests rejection, without concurrent reuse.
			if got := pool.ReleaseSlice(first); got != bytebufferpool.RejectedDuplicate {
				t.Fatalf("recent tombstone = %v; want RejectedDuplicate", got)
			}
			pool.ReleaseSlice(third)
			if got := pool.ReleaseSlice(first); got != bytebufferpool.RejectedForeign {
				t.Fatalf("oldest tombstone = %v; duplicate check must not refresh FIFO", got)
			}
			if first[0] != 0x71 {
				t.Fatal("duplicate or foreign rejection modified storage")
			}
			if got := pool.ReleaseSlice(second); got != bytebufferpool.RejectedDuplicate {
				t.Fatalf("second tombstone = %v; want RejectedDuplicate", got)
			}
		})
	}
}

func TestValidationConcurrentClearReleaseAndStatsPreserveOwners(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%d/zero=%t", mode, zero), func(t *testing.T) {
				config := bytebufferpool.DefaultConfig(mode)
				config.ValidationEnabled = true
				config.MaxValidationTombstones = 3
				config.ZeroOnRelease = zero
				pool, err := bytebufferpool.New(config)
				if err != nil {
					t.Fatal(err)
				}
				const workers = 8
				var wait sync.WaitGroup
				for i := 0; i < workers; i++ {
					wait.Add(1)
					go func() {
						defer wait.Done()
						for j := 0; j < 300; j++ {
							raw := pool.AcquireSlice(64)
							raw[0] = 0x52
							// Clear may advance between Release's Generation reads.
							if got := pool.ReleaseSlice(raw); got != bytebufferpool.Retained && got != bytebufferpool.DroppedStale {
								t.Errorf("valid owner ReleaseSlice() = %v; want Retained or DroppedStale", got)
								return
							}
						}
					}()
				}
				for i := 0; i < 300; i++ {
					pool.Clear()
					stats := pool.Stats()
					if stats.ActiveRawSlices < 0 || stats.ActiveRawSlices > workers || stats.ValidationTombstones < 0 || stats.ValidationTombstones > 3 {
						t.Errorf("concurrent Validation Inventory out of bounds: %+v", stats)
					}
				}
				wait.Wait()
				pool.Clear()
				stats := pool.Stats()
				if stats.ActiveRawSlices != 0 || stats.ValidationTombstones != 0 {
					t.Fatalf("owners or tombstones leaked after final Clear: %+v", stats)
				}
			})
		}
	}
}

func TestValidationReacquiredStorageGetsNewFIFOPosition(t *testing.T) {
	for _, position := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("original-position=%d", position), func(t *testing.T) {
			pool, err := bytebufferpool.New(bytebufferpool.Config{
				Mode: bytebufferpool.Bounded, Classes: []int{64},
				MaxPooledCapacity: 64, MaxRetainedCapacity: 64,
				ValidationEnabled: true, MaxValidationTombstones: 3,
			})
			if err != nil {
				t.Fatal(err)
			}
			original := pool.AcquireSlice(64)
			older, newer, overflow := pool.AcquireSlice(65), pool.AcquireSlice(65), pool.AcquireSlice(65)
			order := [][]byte{older, newer}
			order = append(order[:position], append([][]byte{original}, order[position:]...)...)
			for _, raw := range order {
				pool.ReleaseSlice(raw)
			}
			// There is exactly one retained allocation, so no ordering among
			// retained values is assumed when arranging address reuse.
			reused := pool.AcquireSlice(64)
			if &original[0] != &reused[0] {
				t.Fatal("expected the only retained allocation to be reused")
			}
			pool.ReleaseSlice(reused)
			pool.ReleaseSlice(overflow)
			if got := pool.ReleaseSlice(older); got != bytebufferpool.RejectedForeign {
				t.Fatalf("oldest other tombstone = %v; want RejectedForeign", got)
			}
			if got := pool.ReleaseSlice(reused); got != bytebufferpool.RejectedDuplicate {
				t.Fatalf("new tombstone at reused address = %v; want RejectedDuplicate", got)
			}
			stats := pool.Stats()
			if stats.ActiveRawSlices != 0 || stats.ValidationTombstones != 3 {
				t.Fatalf("address reuse corrupted inventory: %+v", stats)
			}
		})
	}
}
