package bytebufferpool

import "sync/atomic"

// ClassStats reports optional operation counters for one Capacity Class.
type ClassStats struct {
	Capacity int
	Hits     uint64
	Misses   uint64
	Retained uint64
	Dropped  uint64
}

// Stats is a point-in-time view of Pool inventory and optional operations.
type Stats struct {
	RetainedAvailable bool
	RetainedBuffers   int64
	RetainedCapacity  int64

	CountersAvailable bool
	Acquires          uint64
	Hits              uint64
	Misses            uint64
	Releases          uint64
	Retained          uint64
	DroppedFull       uint64
	DroppedOversize   uint64
	DroppedInvalid    uint64
	DroppedStale      uint64
	RejectedForeign   uint64
	RejectedDuplicate uint64
	IgnoredNil        uint64
	ZeroedBytes       uint64
	Classes           []ClassStats
}

type atomicClassCounters struct {
	hits     atomic.Uint64
	misses   atomic.Uint64
	retained atomic.Uint64
	dropped  atomic.Uint64
}

type poolCounters struct {
	acquires          atomic.Uint64
	hits              atomic.Uint64
	misses            atomic.Uint64
	releases          atomic.Uint64
	retained          atomic.Uint64
	droppedFull       atomic.Uint64
	droppedOversize   atomic.Uint64
	droppedInvalid    atomic.Uint64
	droppedStale      atomic.Uint64
	rejectedForeign   atomic.Uint64
	rejectedDuplicate atomic.Uint64
	ignoredNil        atomic.Uint64
	zeroedBytes       atomic.Uint64
	classes           []atomicClassCounters
	capacities        []int
}

func newPoolCounters(capacities []int) *poolCounters {
	return &poolCounters{
		classes:    make([]atomicClassCounters, len(capacities)),
		capacities: append([]int(nil), capacities...),
	}
}

// Stats returns a point-in-time view of Pool inventory and operations.
func (p *Pool) Stats() Stats {
	stats := Stats{}
	if p.config.Mode == Bounded {
		generation := p.current.Load()
		stats.RetainedAvailable = true
		stats.RetainedBuffers = generation.retainedBuffers.Load()
		stats.RetainedCapacity = generation.retainedBytes.Load()
	}
	if p.counters == nil {
		return stats
	}

	counters := p.counters
	stats.CountersAvailable = true
	stats.Acquires = counters.acquires.Load()
	stats.Hits = counters.hits.Load()
	stats.Misses = counters.misses.Load()
	stats.Releases = counters.releases.Load()
	stats.Retained = counters.retained.Load()
	stats.DroppedFull = counters.droppedFull.Load()
	stats.DroppedOversize = counters.droppedOversize.Load()
	stats.DroppedInvalid = counters.droppedInvalid.Load()
	stats.DroppedStale = counters.droppedStale.Load()
	stats.RejectedForeign = counters.rejectedForeign.Load()
	stats.RejectedDuplicate = counters.rejectedDuplicate.Load()
	stats.IgnoredNil = counters.ignoredNil.Load()
	stats.ZeroedBytes = counters.zeroedBytes.Load()
	stats.Classes = make([]ClassStats, len(counters.classes))
	for i := range counters.classes {
		class := &counters.classes[i]
		stats.Classes[i] = ClassStats{
			Capacity: counters.capacities[i],
			Hits:     class.hits.Load(),
			Misses:   class.misses.Load(),
			Retained: class.retained.Load(),
			Dropped:  class.dropped.Load(),
		}
	}
	return stats
}

func (p *Pool) recordAcquire(class int, hit bool) {
	if p.counters == nil {
		return
	}
	p.counters.acquires.Add(1)
	if hit {
		p.counters.hits.Add(1)
		if class >= 0 {
			p.counters.classes[class].hits.Add(1)
		}
		return
	}
	p.counters.misses.Add(1)
	if class >= 0 {
		p.counters.classes[class].misses.Add(1)
	}
}

func (p *Pool) recordRelease(status ReleaseStatus, class int) ReleaseStatus {
	if p.counters == nil {
		return status
	}
	p.counters.releases.Add(1)
	switch status {
	case Retained:
		p.counters.retained.Add(1)
		if class >= 0 {
			p.counters.classes[class].retained.Add(1)
		}
	case DroppedFull:
		p.counters.droppedFull.Add(1)
	case DroppedOversize:
		p.counters.droppedOversize.Add(1)
	case DroppedInvalid:
		p.counters.droppedInvalid.Add(1)
	case DroppedStale:
		p.counters.droppedStale.Add(1)
	case RejectedForeign:
		p.counters.rejectedForeign.Add(1)
	case RejectedDuplicate:
		p.counters.rejectedDuplicate.Add(1)
	case IgnoredNil:
		p.counters.ignoredNil.Add(1)
	}
	if status != Retained && class >= 0 {
		p.counters.classes[class].dropped.Add(1)
	}
	return status
}

func (p *Pool) prepareLeaseRelease(buffer []byte) {
	if !p.config.ZeroOnRelease {
		return
	}
	full := buffer[:cap(buffer)]
	clear(full)
	p.recordZeroed(len(full))
}

func (p *Pool) recordZeroed(bytes int) {
	if p.counters != nil {
		p.counters.zeroedBytes.Add(uint64(bytes))
	}
}
