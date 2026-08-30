package benchmarks

import (
	"bytes"
	"testing"
)

func TestRawAdaptersShareRequestedLengthContract(t *testing.T) {
	for _, adapter := range rawAdapters() {
		t.Run(adapter.Name(), func(t *testing.T) {
			for _, size := range []int{0, 63, 64, 65, 1024, (1 << 20) + 1} {
				for attempt := 0; attempt < 2; attempt++ {
					var borrowed rawBorrowed
					adapter.Acquire(size, &borrowed)
					if len(borrowed.bytes) != size || cap(borrowed.bytes) < size {
						t.Fatalf("Acquire(%d) = len %d/cap %d; want len %d and sufficient capacity", size, len(borrowed.bytes), cap(borrowed.bytes), size)
					}
					if size > 0 {
						borrowed.bytes[0] = 0x11
						borrowed.bytes[size-1] = 0x22
					}
					adapter.Release(&borrowed)
				}
			}
		})
	}
}

func TestBufferAdaptersShareAppendAndResetContract(t *testing.T) {
	want := []byte("first-second-third")
	for _, adapter := range bufferAdapters() {
		t.Run(adapter.Name(), func(t *testing.T) {
			for attempt := 0; attempt < 2; attempt++ {
				borrowed := adapter.Acquire()
				adapter.Write(&borrowed, []byte("first-"))
				adapter.Write(&borrowed, []byte("second-"))
				adapter.Write(&borrowed, []byte("third"))
				if got := adapter.Bytes(&borrowed); !bytes.Equal(got, want) {
					t.Fatalf("Bytes() = %q; want %q", got, want)
				}
				adapter.Release(&borrowed)
			}

			empty := adapter.Acquire()
			if got := adapter.Bytes(&empty); len(got) != 0 {
				t.Fatalf("reacquired Buffer contains %q; want empty", got)
			}
			adapter.Release(&empty)
		})
	}
}
