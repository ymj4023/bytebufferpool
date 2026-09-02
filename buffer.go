package bytebufferpool

import "fmt"

// Buffer is a non-copyable append-oriented value backed by a Lease.
type Buffer struct {
	self     *Buffer
	pool     *Pool
	lease    Lease
	released bool
}

// Buffer returns an empty Buffer with at least initialCapacity bytes of capacity.
// It panics when initialCapacity is invalid.
func (p *Pool) Buffer(initialCapacity int) Buffer {
	buffer, err := p.TryBuffer(initialCapacity)
	if err != nil {
		panic(err)
	}
	return buffer
}

// TryBuffer returns an empty Buffer with at least initialCapacity bytes of capacity.
func (p *Pool) TryBuffer(initialCapacity int) (Buffer, error) {
	if err := p.validateSize(initialCapacity); err != nil {
		return Buffer{}, err
	}
	buffer := Buffer{pool: p}
	lease, err := p.TryAcquire(initialCapacity)
	if err != nil {
		return Buffer{}, err
	}
	lease.storage.buf = lease.storage.buf[:0]
	buffer.lease = lease
	return buffer, nil
}

// Bytes returns the Buffer contents. The result is invalidated by the next
// Buffer mutation or Release.
func (b *Buffer) Bytes() []byte {
	b.checkUsable()
	if b.lease.storage == nil {
		return nil
	}
	return b.lease.storage.buf
}

// Len returns the number of bytes in the Buffer.
func (b *Buffer) Len() int {
	b.checkUsable()
	if b.lease.storage == nil {
		return 0
	}
	return len(b.lease.storage.buf)
}

// Cap returns the capacity of the Buffer's Backing Storage.
func (b *Buffer) Cap() int {
	b.checkUsable()
	if b.lease.storage == nil {
		return 0
	}
	return cap(b.lease.storage.buf)
}

// Grow ensures space for n additional bytes without changing Len.
func (b *Buffer) Grow(n int) error {
	b.checkUsable()
	if n < 0 || n > maxInt()-b.length() {
		return fmt.Errorf("%w: Buffer growth %d", ErrInvalidSize, n)
	}
	return b.ensureCapacity(b.length() + n)
}

// Reset makes the Buffer empty while retaining its current Lease.
func (b *Buffer) Reset() {
	b.checkUsable()
	if b.lease.storage != nil {
		b.lease.storage.buf = b.lease.storage.buf[:0]
	}
}

// Write appends data to the Buffer.
func (b *Buffer) Write(data []byte) (int, error) {
	b.checkUsable()
	if len(data) == 0 {
		return 0, nil
	}
	if len(data) > maxInt()-b.length() {
		return 0, fmt.Errorf("%w: Buffer length overflow", ErrInvalidSize)
	}
	required := b.length() + len(data)
	if required > cap(b.lease.storage.buf) {
		return b.writeWithGrowth(data, required)
	}
	if err := b.ensureCapacity(required); err != nil {
		return 0, err
	}
	b.lease.storage.buf = append(b.lease.storage.buf, data...)
	return len(data), nil
}

func (b *Buffer) writeWithGrowth(data []byte, required int) (int, error) {
	lease, err := b.pool.TryAcquire(b.growthCapacity(required))
	if err != nil {
		return 0, err
	}
	oldLength := b.length()
	copy(lease.storage.buf, b.lease.storage.buf)
	copy(lease.storage.buf[oldLength:], data)
	lease.storage.buf = lease.storage.buf[:required]
	b.lease.Release()
	b.lease = lease
	return len(data), nil
}

// WriteByte appends one byte to the Buffer.
func (b *Buffer) WriteByte(value byte) error {
	b.checkUsable()
	if b.length() == maxInt() {
		return fmt.Errorf("%w: Buffer length overflow", ErrInvalidSize)
	}
	if err := b.ensureCapacity(b.length() + 1); err != nil {
		return err
	}
	b.lease.storage.buf = append(b.lease.storage.buf, value)
	return nil
}

// WriteString appends value to the Buffer.
func (b *Buffer) WriteString(value string) (int, error) {
	b.checkUsable()
	if len(value) == 0 {
		return 0, nil
	}
	if len(value) > maxInt()-b.length() {
		return 0, fmt.Errorf("%w: Buffer length overflow", ErrInvalidSize)
	}
	if err := b.ensureCapacity(b.length() + len(value)); err != nil {
		return 0, err
	}
	b.lease.storage.buf = append(b.lease.storage.buf, value...)
	return len(value), nil
}

// Release transfers the Buffer's Lease back to its Pool.
func (b *Buffer) Release() ReleaseStatus {
	b.checkCopy()
	if b.released {
		return b.pool.recordRelease(RejectedDuplicate, -1)
	}
	b.released = true
	if b.lease.storage == nil {
		return b.pool.recordRelease(IgnoredNil, -1)
	}
	return b.lease.Release()
}

func (b *Buffer) ensureCapacity(required int) error {
	if required == 0 {
		return nil
	}
	if b.lease.storage != nil && required <= cap(b.lease.storage.buf) {
		return nil
	}

	lease, err := b.pool.TryAcquire(b.growthCapacity(required))
	if err != nil {
		return err
	}
	oldLength := b.length()
	if b.lease.storage != nil {
		copy(lease.storage.buf, b.lease.storage.buf)
		lease.storage.buf = lease.storage.buf[:oldLength]
		b.lease.Release()
	} else {
		lease.storage.buf = lease.storage.buf[:0]
	}
	b.lease = lease
	return nil
}

func (b *Buffer) growthCapacity(required int) int {
	if b.pool.classForSize(required) >= 0 {
		return required
	}

	// Design reference: bytes.Buffer uses geometric growth to amortize repeated
	// appends. This Buffer preserves deterministic Capacity Class routing while
	// pooled, applies geometric reservation only when no class can satisfy the
	// request, and clamps that reservation to MaxAcquireSize.
	// https://go.dev/src/bytes/buffer.go
	currentCapacity := 0
	if b.lease.storage != nil {
		currentCapacity = cap(b.lease.storage.buf)
	}
	target := required
	if currentCapacity <= maxInt()/2 && 2*currentCapacity > target {
		target = 2 * currentCapacity
	}
	if limit := b.pool.config.MaxAcquireSize; limit > 0 {
		if required > limit {
			return required
		}
		if target > limit {
			target = limit
		}
	}
	return target
}

func (b *Buffer) length() int {
	if b.lease.storage == nil {
		return 0
	}
	return len(b.lease.storage.buf)
}

func (b *Buffer) checkCopy() {
	if b == nil {
		panic("bytebufferpool: nil Buffer")
	}
	if b.self == nil {
		// Design reference: strings.Builder uses a self-pointer to detect copies.
		// Buffer applies the idea to a Lease owner and keeps independent code.
		// https://go.dev/src/strings/builder.go
		b.self = b
	} else if b.self != b {
		panic("bytebufferpool: illegal copy of Buffer")
	}
}

func (b *Buffer) checkUsable() {
	b.checkCopy()
	if b.released {
		panic("bytebufferpool: use of released Buffer")
	}
	b.lease.checkUsable()
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
