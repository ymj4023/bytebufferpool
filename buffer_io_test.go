package bytebufferpool_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

var (
	_ io.Writer       = (*bytebufferpool.Buffer)(nil)
	_ io.ByteWriter   = (*bytebufferpool.Buffer)(nil)
	_ io.StringWriter = (*bytebufferpool.Buffer)(nil)
	_ io.ReaderFrom   = (*bytebufferpool.Buffer)(nil)
	_ io.WriterTo     = (*bytebufferpool.Buffer)(nil)
)

var errReader = errors.New("reader failed after data")

type dataErrorReader struct {
	done bool
}

func (r *dataErrorReader) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, errReader
	}
	r.done = true
	return copy(buffer, "tail"), errReader
}

type zeroProgressReader struct{}

func (zeroProgressReader) Read([]byte) (int, error) { return 0, nil }

type limitedWriter struct {
	limit int
	err   error
	data  []byte
}

func (w *limitedWriter) Write(buffer []byte) (int, error) {
	n := min(w.limit, len(buffer))
	w.data = append(w.data, buffer[:n]...)
	return n, w.err
}

func TestBufferReadFromAppendsAndPreservesDataBeforeError(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	buffer := pool.Buffer(64)
	defer buffer.Release()
	buffer.WriteString("prefix-")

	n, err := buffer.ReadFrom(strings.NewReader("body"))
	if err != nil || n != 4 {
		t.Fatalf("ReadFrom(body) = %d, %v; want 4, nil", n, err)
	}
	n, err = buffer.ReadFrom(&dataErrorReader{})
	if !errors.Is(err, errReader) || n != 4 {
		t.Fatalf("ReadFrom(data+error) = %d, %v; want 4, errReader", n, err)
	}
	if got := string(buffer.Bytes()); got != "prefix-bodytail" {
		t.Fatalf("Buffer contents = %q; want %q", got, "prefix-bodytail")
	}
}

func TestBufferReadFromStopsZeroProgressAndPreservesOnGrowthFailure(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.Config{
		Mode:              bytebufferpool.Fast,
		Classes:           []int{64},
		MaxPooledCapacity: 64,
		MaxAcquireSize:    64,
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	empty := pool.Buffer(0)
	if n, err := empty.ReadFrom(zeroProgressReader{}); n != 0 || !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("ReadFrom(zero progress) = %d, %v; want 0, io.ErrNoProgress", n, err)
	}
	empty.Release()

	full := pool.Buffer(64)
	defer full.Release()
	full.Write(bytes.Repeat([]byte{'x'}, 64))
	want := append([]byte(nil), full.Bytes()...)
	if n, err := full.ReadFrom(strings.NewReader("overflow")); n != 0 || !errors.Is(err, bytebufferpool.ErrInvalidSize) {
		t.Fatalf("ReadFrom(over limit) = %d, %v; want 0, ErrInvalidSize", n, err)
	}
	if !bytes.Equal(full.Bytes(), want) {
		t.Fatal("ReadFrom growth failure mutated Buffer")
	}
}

func TestBufferWriteToConsumesOnlyConfirmedPrefix(t *testing.T) {
	pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	t.Run("complete", func(t *testing.T) {
		buffer := pool.Buffer(64)
		defer buffer.Release()
		buffer.WriteString("complete")
		var output bytes.Buffer
		n, err := buffer.WriteTo(&output)
		if err != nil || n != 8 || output.String() != "complete" || buffer.Len() != 0 {
			t.Fatalf("WriteTo complete = %d, %v, output %q, remaining %q", n, err, output.String(), buffer.Bytes())
		}
	})

	t.Run("short write", func(t *testing.T) {
		buffer := pool.Buffer(64)
		defer buffer.Release()
		buffer.WriteString("abcdef")
		writer := &limitedWriter{limit: 2}
		n, err := buffer.WriteTo(writer)
		if n != 2 || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("WriteTo short = %d, %v; want 2, io.ErrShortWrite", n, err)
		}
		if string(writer.data) != "ab" || string(buffer.Bytes()) != "cdef" {
			t.Fatalf("short write output=%q remaining=%q; want %q/%q", writer.data, buffer.Bytes(), "ab", "cdef")
		}
	})

	t.Run("partial error", func(t *testing.T) {
		buffer := pool.Buffer(64)
		defer buffer.Release()
		buffer.WriteString("abcdef")
		writeErr := errors.New("writer stopped")
		writer := &limitedWriter{limit: 3, err: writeErr}
		n, err := buffer.WriteTo(writer)
		if n != 3 || !errors.Is(err, writeErr) {
			t.Fatalf("WriteTo partial error = %d, %v; want 3, writeErr", n, err)
		}
		if string(writer.data) != "abc" || string(buffer.Bytes()) != "def" {
			t.Fatalf("partial output=%q remaining=%q; want %q/%q", writer.data, buffer.Bytes(), "abc", "def")
		}
	})
}
