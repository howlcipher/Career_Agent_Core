package util

import (
	"bytes"
	"io"
	"sync"
)

// BufferPool is a shared sync.Pool of bytes.Buffer to reduce allocations
// during I/O operations like HTTP responses.
var BufferPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate a 4KB buffer by default.
		b := new(bytes.Buffer)
		b.Grow(4096)
		return b
	},
}

// GetBuffer returns a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	return BufferPool.Get().(*bytes.Buffer)
}

// PutBuffer resets the buffer and returns it to the pool.
// It discards large buffers to avoid holding too much memory.
func PutBuffer(b *bytes.Buffer) {
	if b.Cap() > 1024*1024 { // 1MB limit for returning to pool
		return
	}
	b.Reset()
	BufferPool.Put(b)
}

// ReadAll uses the BufferPool to read from r into a buffer and returns a copy of the bytes.
// Note: It returns a newly allocated []byte of exactly the right size, but avoids
// intermediate allocations that io.ReadAll would make while growing the buffer.
func ReadAll(r io.Reader) ([]byte, error) {
	b := GetBuffer()
	defer PutBuffer(b)

	_, err := b.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	// Copy the bytes so the caller owns them and we can reuse the buffer safely.
	res := make([]byte, b.Len())
	copy(res, b.Bytes())
	return res, nil
}
