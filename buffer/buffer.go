package buffer

import (
	"github.com/inelpandzic/simpledb/file"
)

// Buffer represents a single buffer in the buffer pool.
// It wraps a Page with additional metadata for buffer management.
type Buffer struct {
	page     *file.Page
	block    *file.BlockID
	pinCount int
	dirty    bool
	logLSN   int // Log sequence number for recovery
}

// NewBuffer creates a new buffer with the given page size.
func NewBuffer(blockSize int) *Buffer {
	return &Buffer{
		page:     file.NewPage(blockSize),
		pinCount: 0,
		dirty:    false,
		logLSN:   -1,
	}
}

// Page returns the underlying page.
func (b *Buffer) Page() *file.Page {
	return b.page
}

// Block returns the block ID this buffer is associated with.
func (b *Buffer) Block() *file.BlockID {
	return b.block
}

// Pin increments the pin count to prevent buffer replacement.
func (b *Buffer) Pin() {
	b.pinCount++
}

// Unpin decrements the pin count.
func (b *Buffer) Unpin() {
	if b.pinCount > 0 {
		b.pinCount--
	}
}

// IsPinned returns true if the buffer is currently pinned.
func (b *Buffer) IsPinned() bool {
	return b.pinCount > 0
}

// SetDirty marks the buffer as modified.
func (b *Buffer) SetDirty(logLSN int) {
	b.dirty = true
	b.logLSN = logLSN
}

// IsDirty returns true if the buffer has been modified.
func (b *Buffer) IsDirty() bool {
	return b.dirty
}

// LogLSN returns the log sequence number associated with this buffer.
func (b *Buffer) LogLSN() int {
	return b.logLSN
}

// assignToBlock associates this buffer with a specific block.
func (b *Buffer) assignToBlock(block *file.BlockID) {
	b.block = block
	b.pinCount = 0
	b.dirty = false
	b.logLSN = -1
}
