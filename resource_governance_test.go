package bytebufferpool_test

import (
	"fmt"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestResourceGovernanceContractsComposeAcrossConfigurations(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, zero := range []bool{false, true} {
			for _, counters := range []bool{false, true} {
				t.Run(fmt.Sprintf("mode=%d/zero=%t/counters=%t", mode, zero, counters), func(t *testing.T) {
					config := bytebufferpool.DefaultConfig(mode)
					config.Classes = []int{64}
					config.MaxPooledCapacity = 128
					config.ValidationEnabled = true
					config.MaxValidationTombstones = 1
					config.ZeroOnRelease = zero
					config.StatsEnabled = counters
					pool, err := bytebufferpool.New(config)
					if err != nil {
						t.Fatal(err)
					}
					config.Classes[0] = 1
					config.MaxValidationTombstones = 999 // Caller configuration is no longer Pool policy.
					idle, active := pool.AcquireSlice(64), pool.AcquireSlice(64)
					gap, oversize := pool.AcquireSlice(100), pool.AcquireSlice(129)
					if got := pool.Stats().ActiveRawSlices; got != 4 {
						t.Fatalf("active above limit = %d; want 4", got)
					}
					if pool.ReleaseSlice(idle) != bytebufferpool.Retained || pool.ReleaseSlice(oversize) != bytebufferpool.DroppedOversize {
						t.Fatal("unexpected seed release outcomes")
					}
					pool.Clear()
					stats := pool.Stats()
					if stats.Generation != 1 || stats.ActiveRawSlices != 2 || stats.ValidationTombstones != 0 || stats.MaxValidationTombstones != 1 || stats.RetainedCapacity != 0 {
						t.Fatalf("post-Clear combined state: %+v", stats)
					}
					oversize[0] = 0x6a // Deliberate old alias; rejected without mutation.
					if got := pool.ReleaseSlice(oversize); got != bytebufferpool.RejectedForeign || oversize[0] != 0x6a {
						t.Fatalf("cleared alias = %v, byte %#x", got, oversize[0])
					}
					fillBytes(active, 0x41)
					fillBytes(gap, 0x42)
					if pool.ReleaseSlice(active) != bytebufferpool.Retained || pool.ReleaseSlice(gap) != bytebufferpool.DroppedUnpooled {
						t.Fatal("Clear lost active provenance or class-gap classification")
					}
					if zero {
						assertAllZero(t, "active", active)
						assertAllZero(t, "gap", gap)
					}
					stats = pool.Stats()
					if !stats.ValidationAvailable || stats.ActiveRawSlices != 0 || stats.ValidationTombstones != 1 || stats.Generation != 1 {
						t.Fatalf("final Validation Inventory: %+v", stats)
					}
					if mode == bytebufferpool.Bounded {
						if !stats.RetainedAvailable || stats.RetainedStorageCount != 1 || stats.RetainedCapacity != 64 || len(stats.ClassInventory) != 1 || stats.ClassInventory[0] != (bytebufferpool.ClassInventory{Capacity: 64, IdleStorageCount: 1, RetainedCapacity: 64}) {
							t.Fatalf("final Bounded inventory: %+v", stats)
						}
					} else if stats.RetainedAvailable || len(stats.ClassInventory) != 0 {
						t.Fatal("Fast invented retained inventory")
					}
					wantReleases, wantUnpooled := uint64(0), uint64(0)
					if counters {
						wantReleases, wantUnpooled = 5, 1
					}
					if stats.CountersAvailable != counters || stats.Releases != wantReleases || stats.DroppedUnpooled != wantUnpooled {
						t.Fatalf("optional counters: %+v", stats)
					}
				})
			}
		}
	}
}
