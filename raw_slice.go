package bytebufferpool

import "unsafe"

const releasedDiagnosticByte = 0xa5

type rawRecord struct {
	capacity          int
	active            bool
	previousTombstone uintptr
	nextTombstone     uintptr
}

// AcquireSlice returns a low-level Raw Slice and panics when size is invalid.
// Unlike Lease, a Raw Slice carries no Generation and cannot prevent stale
// aliases from releasing a backing array that has since been reacquired.
func (p *Pool) AcquireSlice(size int) []byte {
	buffer, err := p.TryAcquireSlice(size)
	if err != nil {
		panic(err)
	}
	return buffer
}

// TryAcquireSlice returns a low-level Raw Slice of size bytes.
func (p *Pool) TryAcquireSlice(size int) ([]byte, error) {
	if err := p.validateSize(size); err != nil {
		return nil, err
	}
	if size == 0 {
		p.recordAcquire(-1, false)
		return nil, nil
	}

	storage, _ := p.acquireStorage(size)
	buffer := storage.buf
	storage.buf = nil
	storage.class = -1
	storage.active.Store(false)
	p.rawWrappers.Put(storage)

	if p.config.ValidationEnabled {
		p.registerRaw(buffer)
	}
	return buffer, nil
}

// ReleaseSlice releases a Raw Slice to the current Pool Generation.
// Enhanced validation detects observable ownership mistakes, but cannot
// eliminate the ABA ambiguity created by mutable slice aliases.
// An inactive record evicted by the FIFO limit or discarded by Clear is
// reported as RejectedForeign instead of RejectedDuplicate, without mutation.
func (p *Pool) ReleaseSlice(buffer []byte) ReleaseStatus {
	if buffer == nil {
		return p.recordRelease(IgnoredNil, -1)
	}

	releaseCapacity := cap(buffer)
	if p.config.ValidationEnabled {
		status, proceed, owned, originalCapacity := p.validateRawRelease(buffer)
		releaseCapacity = originalCapacity
		if !proceed {
			if owned {
				p.prepareRawRelease(buffer, releaseCapacity)
			}
			return p.recordRelease(status, -1)
		}
	}
	p.prepareRawRelease(buffer, releaseCapacity)

	capacity := cap(buffer)
	if capacity > p.config.MaxPooledCapacity {
		return p.recordRelease(DroppedOversize, -1)
	}
	class := p.classForCapacity(capacity)
	if class < 0 {
		if capacity == 0 {
			return p.recordRelease(DroppedInvalid, -1)
		}
		return p.recordRelease(DroppedUnpooled, -1)
	}

	storage := p.rawWrapper()
	storage.buf = buffer
	storage.class = class
	status := p.release(storage, p.current.Load().id)
	if status != Retained {
		storage.buf = nil
		storage.class = -1
		p.rawWrappers.Put(storage)
	}
	return status
}

func (p *Pool) rawWrapper() *backingStorage {
	if value := p.rawWrappers.Get(); value != nil {
		return value.(*backingStorage)
	}
	return &backingStorage{}
}

func (p *Pool) registerRaw(buffer []byte) {
	key := rawKey(buffer)
	p.validationMu.Lock()
	record, exists := p.rawRecords[key]
	if record.active {
		p.validationMu.Unlock()
		panic("bytebufferpool: backing storage handed to two live Raw Slice owners")
	}
	if exists {
		p.removeValidationTombstoneLocked(record)
	}
	p.rawRecords[key] = rawRecord{capacity: cap(buffer), active: true}
	p.activeRawSlices++
	p.validationMu.Unlock()
}

func (p *Pool) validateRawRelease(buffer []byte) (status ReleaseStatus, proceed, owned bool, originalCapacity int) {
	key := rawKey(buffer)
	p.validationMu.Lock()
	record, ok := p.rawRecords[key]
	if !ok {
		p.validationMu.Unlock()
		return RejectedForeign, false, false, cap(buffer)
	}
	if !record.active {
		p.validationMu.Unlock()
		return RejectedDuplicate, false, false, record.capacity
	}
	p.activeRawSlices--
	p.addValidationTombstoneLocked(key, record)
	p.validationMu.Unlock()

	if cap(buffer) != record.capacity {
		return DroppedInvalid, false, true, record.capacity
	}
	return Retained, true, true, record.capacity
}

func (p *Pool) addValidationTombstoneLocked(key uintptr, record rawRecord) {
	record.active = false
	record.previousTombstone = p.validationTombstoneTail
	record.nextTombstone = 0
	if p.validationTombstoneTail == 0 {
		p.validationTombstoneHead = key
	} else {
		tail := p.rawRecords[p.validationTombstoneTail]
		tail.nextTombstone = key
		p.rawRecords[p.validationTombstoneTail] = tail
	}
	p.validationTombstoneTail = key
	p.rawRecords[key] = record
	p.validationTombstones++

	for p.validationTombstones > int64(p.config.MaxValidationTombstones) {
		p.evictOldestValidationTombstoneLocked()
	}
}

func (p *Pool) removeValidationTombstoneLocked(record rawRecord) {
	if record.previousTombstone == 0 {
		p.validationTombstoneHead = record.nextTombstone
	} else {
		previous := p.rawRecords[record.previousTombstone]
		previous.nextTombstone = record.nextTombstone
		p.rawRecords[record.previousTombstone] = previous
	}
	if record.nextTombstone == 0 {
		p.validationTombstoneTail = record.previousTombstone
	} else {
		next := p.rawRecords[record.nextTombstone]
		next.previousTombstone = record.previousTombstone
		p.rawRecords[record.nextTombstone] = next
	}
	p.validationTombstones--
}

func (p *Pool) evictOldestValidationTombstoneLocked() {
	key := p.validationTombstoneHead
	record := p.rawRecords[key]
	p.removeValidationTombstoneLocked(record)
	delete(p.rawRecords, key)
}

func (p *Pool) clearValidationTombstones() {
	if !p.config.ValidationEnabled {
		return
	}
	p.validationMu.Lock()
	activeRecords := make(map[uintptr]rawRecord)
	for key, record := range p.rawRecords {
		if record.active {
			record.previousTombstone = 0
			record.nextTombstone = 0
			activeRecords[key] = record
		}
	}
	p.rawRecords = activeRecords
	p.validationTombstoneHead = 0
	p.validationTombstoneTail = 0
	p.validationTombstones = 0
	p.validationMu.Unlock()
}

func rawKey(buffer []byte) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(buffer)))
}

func (p *Pool) prepareRawRelease(buffer []byte, originalCapacity int) {
	full := buffer[:cap(buffer)]
	if originalCapacity > cap(buffer) {
		full = unsafe.Slice(unsafe.SliceData(buffer), originalCapacity)
	}
	if p.config.ZeroOnRelease {
		clear(full)
		p.recordZeroed(len(full))
		return
	}
	if p.config.ValidationEnabled {
		for i := range full {
			full[i] = releasedDiagnosticByte
		}
	}
}
