package bytebufferpool_test

import (
	"fmt"

	bytebufferpool "github.com/ymj4023/bytebufferpool"
)

func ExamplePool_Acquire() {
	pool, _ := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	lease := pool.Acquire(5)
	copy(lease.Bytes(), "hello")
	fmt.Println(string(lease.Bytes()))
	fmt.Println(lease.Release())

	// Output:
	// hello
	// Retained
}

func ExamplePool_AcquireSlice() {
	pool, _ := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	buffer := pool.AcquireSlice(5)
	copy(buffer, "hello")
	fmt.Println(string(buffer))
	fmt.Println(pool.ReleaseSlice(buffer))

	// Output:
	// hello
	// Retained
}

func ExamplePool_Buffer() {
	pool, _ := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
	buffer := pool.Buffer(64)
	_, _ = buffer.WriteString("hello")
	_ = buffer.WriteByte(' ')
	_, _ = buffer.Write([]byte("world"))
	fmt.Println(string(buffer.Bytes()))
	fmt.Println(buffer.Release())

	// Output:
	// hello world
	// Retained
}

func ExamplePool_bounded() {
	config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
	config.Classes = []int{64}
	config.MaxPooledCapacity = 64
	config.MaxRetainedCapacity = 64
	pool, _ := bytebufferpool.New(config)

	first := pool.Acquire(64)
	second := pool.Acquire(64)
	fmt.Println(first.Release())
	fmt.Println(second.Release())
	stats := pool.Stats()
	fmt.Println(stats.RetainedBuffers, stats.RetainedCapacity)

	// Output:
	// Retained
	// DroppedFull
	// 1 64
}
