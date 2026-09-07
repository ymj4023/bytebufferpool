package bytebufferpool_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestStatsGenerationAdvancesWithoutOptionalCounters(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		for _, counters := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%d/counters=%t", mode, counters), func(t *testing.T) {
				config := bytebufferpool.DefaultConfig(mode)
				config.StatsEnabled = counters
				pool, err := bytebufferpool.New(config)
				if err != nil {
					t.Fatal(err)
				}
				for generation := uint64(0); generation < 4; generation++ {
					stats := pool.Stats()
					if stats.Generation != generation {
						t.Fatalf("Generation = %d; want %d", stats.Generation, generation)
					}
					if mode == bytebufferpool.Fast && (stats.RetainedAvailable || len(stats.ClassInventory) != 0) {
						t.Fatal("Fast Stats must not estimate retained inventory")
					}
					pool.Clear()
				}
			})
		}
	}
}

func TestBoundedClassInventoryTracksCurrentIdleStorageWithoutCounters(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode: bytebufferpool.Bounded, Classes: []int{64, 128},
		MaxPooledCapacity: 128, MaxRetainedCapacity: 192,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, second, overflow := pool.Acquire(64), pool.Acquire(128), pool.Acquire(64)
	first.Release()
	second.Release()
	if got := overflow.Release(); got != bytebufferpool.DroppedFull {
		t.Fatalf("over-budget Release = %v; want DroppedFull", got)
	}
	want := []bytebufferpool.ClassInventory{
		{Capacity: 64, IdleStorageCount: 1, RetainedCapacity: 64},
		{Capacity: 128, IdleStorageCount: 1, RetainedCapacity: 128},
	}
	stats := pool.Stats()
	if !stats.RetainedAvailable || stats.CountersAvailable || len(stats.Classes) != 0 || !reflect.DeepEqual(stats.ClassInventory, want) || stats.RetainedStorageCount != 2 || stats.RetainedCapacity != 192 {
		t.Fatalf("inventory must report current retained storage, not release history: %+v", stats)
	}
	borrowed := pool.Acquire(64)
	stats = pool.Stats()
	want[0] = bytebufferpool.ClassInventory{Capacity: 64}
	if !reflect.DeepEqual(stats.ClassInventory, want) || stats.RetainedStorageCount != 1 || stats.RetainedCapacity != 128 {
		t.Fatalf("inventory still counts borrowed storage: %+v", stats)
	}
	borrowed.Release()
}

func TestClassInventoryCoversConfiguredClassesAndIsAnIndependentSnapshot(t *testing.T) {
	for _, custom := range []bool{false, true} {
		for _, counters := range []bool{false, true} {
			t.Run(fmt.Sprintf("custom=%t/counters=%t", custom, counters), func(t *testing.T) {
				config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
				config.StatsEnabled = counters
				if custom {
					config.Classes = []int{17, 91, 257}
				}
				pool, err := bytebufferpool.New(config)
				if err != nil {
					t.Fatal(err)
				}
				for _, size := range config.Classes {
					lease := pool.Acquire(size)
					lease.Release()
				}
				stats := pool.Stats()
				if len(stats.ClassInventory) != len(config.Classes) || stats.CountersAvailable != counters {
					t.Fatalf("missing configured classes or wrong counter availability: %+v", stats)
				}
				var capacity int64
				for i, class := range stats.ClassInventory {
					if class.Capacity != config.Classes[i] || class.IdleStorageCount != 1 || class.RetainedCapacity != int64(config.Classes[i]) {
						t.Fatalf("class %d = %+v; want one idle storage with capacity %d", i, class, config.Classes[i])
					}
					capacity += class.RetainedCapacity
				}
				if stats.RetainedCapacity != capacity || stats.RetainedStorageCount != int64(len(config.Classes)) {
					t.Fatalf("global inventory does not reconcile: %+v", stats)
				}
				previous := pool.Stats()
				wantPrevious := append([]bytebufferpool.ClassInventory(nil), previous.ClassInventory...)
				stats.ClassInventory[0].IdleStorageCount = 99
				if !reflect.DeepEqual(previous.ClassInventory, wantPrevious) {
					t.Fatal("caller mutation changed another existing snapshot")
				}
				if pool.Stats().ClassInventory[0].IdleStorageCount != 1 {
					t.Fatal("caller mutation changed Pool inventory")
				}
				pool.Clear()
				cleared := pool.Stats()
				if cleared.Generation != 1 || cleared.RetainedStorageCount != 0 || cleared.RetainedCapacity != 0 {
					t.Fatalf("Clear did not reset current inventory: %+v", cleared)
				}
				for _, class := range cleared.ClassInventory {
					if class.IdleStorageCount != 0 || class.RetainedCapacity != 0 {
						t.Fatalf("Clear left idle class inventory: %+v", class)
					}
				}
				if !reflect.DeepEqual(previous.ClassInventory, wantPrevious) {
					t.Fatal("later Stats or Clear changed previously returned Class Inventory")
				}
			})
		}
	}
}

func TestClassInventoryReconcilesDuringConcurrentReuseAndClear(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Bounded))
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for _, size := range []int{64, 128, 256, 512} {
		wait.Add(1)
		go func(size int) {
			defer wait.Done()
			for i := 0; i < 1000; i++ {
				lease := pool.Acquire(size)
				lease.Release()
			}
		}(size)
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		for i := 0; i < 100; i++ {
			pool.Clear()
		}
	}()
	var previous uint64
	for i := 0; i < 1000; i++ {
		stats := pool.Stats()
		if stats.Generation < previous || stats.Generation > 100 {
			t.Errorf("Generation = %d; previous %d, maximum 100", stats.Generation, previous)
		}
		previous = stats.Generation
		var count, capacity int64
		for _, class := range stats.ClassInventory {
			count += class.IdleStorageCount
			capacity += class.RetainedCapacity
		}
		if count != stats.RetainedStorageCount || capacity != stats.RetainedCapacity {
			t.Errorf("class inventory does not reconcile within snapshot: %+v", stats)
		}
	}
	wait.Wait()
	if got := pool.Stats().Generation; got != 100 {
		t.Fatalf("final Generation = %d; want 100", got)
	}
}
