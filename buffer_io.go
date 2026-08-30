package bytebufferpool

import (
	"fmt"
	"io"
)

const (
	readGrowth               = 512
	maxConsecutiveEmptyReads = 100
)

// ReadFrom appends data from reader until EOF or an error.
func (b *Buffer) ReadFrom(reader io.Reader) (int64, error) {
	b.checkUsable()
	var total int64
	emptyReads := 0

	for {
		if b.lease.storage == nil || len(b.lease.storage.buf) == cap(b.lease.storage.buf) {
			target, err := b.nextReadCapacity()
			if err != nil {
				return total, err
			}
			if err := b.ensureCapacity(target); err != nil {
				return total, err
			}
		}

		length := len(b.lease.storage.buf)
		available := b.lease.storage.buf[length:cap(b.lease.storage.buf)]
		n, err := reader.Read(available)
		if n < 0 || n > len(available) {
			return total, fmt.Errorf("bytebufferpool: Reader returned invalid count %d for %d-byte destination", n, len(available))
		}
		if n > 0 {
			b.lease.storage.buf = b.lease.storage.buf[:length+n]
			total += int64(n)
			emptyReads = 0
		} else if err == nil {
			emptyReads++
			if emptyReads >= maxConsecutiveEmptyReads {
				return total, io.ErrNoProgress
			}
		}

		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

// WriteTo writes Buffer contents and removes only the confirmed written prefix.
func (b *Buffer) WriteTo(writer io.Writer) (int64, error) {
	b.checkUsable()
	if b.length() == 0 {
		return 0, nil
	}

	buffer := b.lease.storage.buf
	n, err := writer.Write(buffer)
	if n < 0 || n > len(buffer) {
		return 0, fmt.Errorf("bytebufferpool: Writer returned invalid count %d for %d-byte source", n, len(buffer))
	}
	if n > 0 {
		copy(buffer, buffer[n:])
		b.lease.storage.buf = buffer[:len(buffer)-n]
	}
	if err != nil {
		return int64(n), err
	}
	if n != len(buffer) {
		return int64(n), io.ErrShortWrite
	}
	return int64(n), nil
}

func (b *Buffer) nextReadCapacity() (int, error) {
	length := b.length()
	if length == maxInt() {
		return 0, fmt.Errorf("%w: Buffer length overflow", ErrInvalidSize)
	}
	target := length + readGrowth
	if target < length {
		return 0, fmt.Errorf("%w: Buffer length overflow", ErrInvalidSize)
	}
	if limit := b.pool.config.MaxAcquireSize; limit > 0 && target > limit {
		target = limit
	}
	if target <= length {
		return 0, fmt.Errorf("%w: Buffer growth exceeds limit", ErrInvalidSize)
	}
	return target, nil
}
