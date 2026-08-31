package bytebufferpool_test

import (
	"bytes"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func FuzzPoolConfiguration(f *testing.F) {
	f.Add(uint8(0), 64, 128, 128, int64(0), 0)
	f.Add(uint8(1), 64, 128, 128, int64(1024), 1024)
	f.Add(uint8(1), -1, 0, -1, int64(-1), -1)

	f.Fuzz(func(t *testing.T, mode uint8, first, second, cutoff int, budget int64, acquireLimit int) {
		classes := []int{first, second}
		pool, err := bytebufferpool.New(bytebufferpool.Config{
			Mode:              bytebufferpool.Mode(mode),
			Classes:           classes,
			MaxPooledCapacity: cutoff,
			MaxRetainedBytes:  budget,
			MaxAcquireSize:    acquireLimit,
		})
		if err != nil {
			return
		}

		classes[0] = 0
		classes[1] = 0
		if _, err := pool.TryAcquire(-1); err == nil {
			t.Fatal("valid Pool accepted a negative acquisition after caller mutated Config.Classes")
		}
	})
}

func FuzzBufferOperations(f *testing.F) {
	f.Add([]byte("write-reset-grow-write"))
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})

	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 4096 {
			t.Skip()
		}
		pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
		if err != nil {
			t.Fatalf("New(): %v", err)
		}
		buffer := pool.Buffer(0)
		model := make([]byte, 0)

		for _, operation := range operations {
			switch operation % 4 {
			case 0:
				if err := buffer.WriteByte(operation); err != nil {
					t.Fatalf("WriteByte(): %v", err)
				}
				model = append(model, operation)
			case 1:
				data := []byte{operation, operation ^ 0xff}
				if _, err := buffer.Write(data); err != nil {
					t.Fatalf("Write(): %v", err)
				}
				model = append(model, data...)
			case 2:
				buffer.Reset()
				model = model[:0]
			case 3:
				if err := buffer.Grow(int(operation & 15)); err != nil {
					t.Fatalf("Grow(): %v", err)
				}
			}
			if !bytes.Equal(buffer.Bytes(), model) {
				t.Fatalf("Buffer.Bytes() = %x; model = %x", buffer.Bytes(), model)
			}
		}

		if got := buffer.Release(); got != bytebufferpool.Retained && got != bytebufferpool.IgnoredNil {
			t.Fatalf("Release() = %v; want Retained or IgnoredNil", got)
		}
		if got := buffer.Release(); got != bytebufferpool.RejectedDuplicate {
			t.Fatalf("duplicate Release() = %v; want RejectedDuplicate", got)
		}
	})
}
