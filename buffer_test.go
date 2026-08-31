package bytebufferpool_test

import (
	"bytes"
	"errors"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func TestBufferWritesAndResetsAcrossBackends(t *testing.T) {
	for _, mode := range []bytebufferpool.Mode{bytebufferpool.Fast, bytebufferpool.Bounded} {
		config := bytebufferpool.Config{
			Mode:              mode,
			Classes:           []int{64, 128},
			MaxPooledCapacity: 128,
		}
		if mode == bytebufferpool.Bounded {
			config.MaxRetainedCapacity = 256
		}
		pool, err := bytebufferpool.New(config)
		if err != nil {
			t.Fatalf("New(%v): %v", mode, err)
		}

		buffer := pool.Buffer(65)
		if buffer.Len() != 0 || buffer.Cap() != 128 {
			t.Fatalf("Buffer(65) = len %d/cap %d; want 0/128", buffer.Len(), buffer.Cap())
		}
		if n, err := buffer.WriteString("hello"); err != nil || n != 5 {
			t.Fatalf("WriteString() = %d, %v; want 5, nil", n, err)
		}
		if err := buffer.WriteByte(' '); err != nil {
			t.Fatalf("WriteByte(): %v", err)
		}
		if n, err := buffer.Write([]byte("world")); err != nil || n != 5 {
			t.Fatalf("Write() = %d, %v; want 5, nil", n, err)
		}
		if got := buffer.Bytes(); !bytes.Equal(got, []byte("hello world")) {
			t.Fatalf("Buffer.Bytes() = %q; want %q", got, "hello world")
		}

		buffer.Reset()
		if buffer.Len() != 0 || buffer.Cap() != 128 {
			t.Fatalf("Buffer after Reset = len %d/cap %d; want 0/128", buffer.Len(), buffer.Cap())
		}
		if got := buffer.Release(); got != bytebufferpool.Retained {
			t.Fatalf("Buffer.Release() = %v; want Retained", got)
		}
	}
}

func TestBufferGrowthReleasesOldLease(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:                bytebufferpool.Bounded,
		Classes:             []int{64, 128},
		MaxPooledCapacity:   128,
		MaxRetainedCapacity: 256,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	buffer := pool.Buffer(64)
	if _, err := buffer.Write(make([]byte, 65)); err != nil {
		t.Fatalf("Write growth: %v", err)
	}

	stats := pool.Stats()
	if stats.RetainedBuffers != 1 || stats.RetainedCapacity != 64 {
		t.Fatalf("inventory after growth = %d buffers/%d bytes; want released old Lease 1/64", stats.RetainedBuffers, stats.RetainedCapacity)
	}
	if buffer.Cap() != 128 || buffer.Len() != 65 {
		t.Fatalf("grown Buffer = len %d/cap %d; want 65/128", buffer.Len(), buffer.Cap())
	}
	if got := buffer.Release(); got != bytebufferpool.Retained {
		t.Fatalf("Buffer.Release() = %v; want Retained", got)
	}
}

func TestBufferGrowthFailurePreservesContent(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		MaxAcquireSize:    64,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	buffer := pool.Buffer(64)
	defer buffer.Release()
	if _, err := buffer.WriteString("unchanged"); err != nil {
		t.Fatalf("seed WriteString(): %v", err)
	}
	want := append([]byte(nil), buffer.Bytes()...)

	if n, err := buffer.Write(make([]byte, 64)); n != 0 || !errors.Is(err, bytebufferpool.ErrInvalidSize) {
		t.Fatalf("oversize Write() = %d, %v; want 0, ErrInvalidSize", n, err)
	}
	if !bytes.Equal(buffer.Bytes(), want) || buffer.Cap() != 64 {
		t.Fatalf("failed growth mutated Buffer: bytes=%q cap=%d; want %q cap=64", buffer.Bytes(), buffer.Cap(), want)
	}
}

func TestBufferRejectsCopyAndPostReleaseUse(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	buffer := pool.Buffer(64)
	buffer.Bytes()
	copied := buffer

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("copied Buffer did not panic")
			}
		}()
		copied.Len()
	}()

	if got := buffer.Release(); got != bytebufferpool.Retained {
		t.Fatalf("first Release() = %v; want Retained", got)
	}
	if got := buffer.Release(); got != bytebufferpool.RejectedDuplicate {
		t.Fatalf("second Release() = %v; want RejectedDuplicate", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Buffer.Bytes() after Release did not panic")
		}
	}()
	buffer.Bytes()
}

func TestEmptyBufferDoesNotAllocateAndInvalidCapacityIsRejected(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		MaxAcquireSize:    64,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	for _, initial := range []int{-1, 65} {
		if _, err := pool.TryBuffer(initial); !errors.Is(err, bytebufferpool.ErrInvalidSize) {
			t.Errorf("TryBuffer(%d) error = %v; want ErrInvalidSize", initial, err)
		}
	}

	buffer := pool.Buffer(0)
	if err := buffer.Grow(0); err != nil {
		t.Fatalf("Grow(0): %v", err)
	}
	if buffer.Len() != 0 || buffer.Cap() != 64 || buffer.Bytes() == nil {
		t.Fatalf("empty Buffer Lease = len=%d cap=%d bytes=%#v; want len 0/cap 64/non-nil", buffer.Len(), buffer.Cap(), buffer.Bytes())
	}
	if got := buffer.Release(); got != bytebufferpool.Retained {
		t.Fatalf("empty Buffer Release() = %v; want Retained", got)
	}
}

func TestBufferCopyBeforeFirstUseCannotOutliveOriginal(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	original := pool.Buffer(64)
	copied := original
	if got := original.Release(); got != bytebufferpool.Retained {
		t.Fatalf("original Release() = %v; want Retained", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Buffer copied before first use wrote after original Release")
		}
	}()
	_ = copied.WriteByte(0x2a)
}
