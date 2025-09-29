package buffer

import (
	"errors"
	"sync"
	"time"

	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
)

var (
	ErrBufferPoolFull = errors.New("buffer pool is full")
	ErrBufferTimeout  = errors.New("timeout waiting for buffer")
)

// BufferMgr manages a pool of buffers using LRU replacement strategy.
type BufferMgr struct {
	buffers    []*Buffer
	numBuffers int
	available  int
	fm         *file.FileMgr
	lm         *log.LogMgr

	mu sync.Mutex
}

// NewBufferMgr creates a new buffer manager with the specified pool size.
func NewBufferMgr(fm *file.FileMgr, lm *log.LogMgr, numBuffers int) *BufferMgr {
	buffers := make([]*Buffer, numBuffers)
	for i := 0; i < numBuffers; i++ {
		buffers[i] = NewBuffer(fm.BlockSize)
	}

	return &BufferMgr{
		buffers:    buffers,
		numBuffers: numBuffers,
		available:  numBuffers,
		fm:         fm,
		lm:         lm,
	}
}

// Pin assigns a buffer to the specified block and pins it.
// If the block is not in the buffer pool, it loads it from disk.
func (bm *BufferMgr) Pin(block *file.BlockID) (*Buffer, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check if block is already in buffer pool
	if buf := bm.findExistingBuffer(block); buf != nil {
		if !buf.IsPinned() {
			bm.available--
		}
		buf.Pin()
		return buf, nil
	}

	// Find an available buffer
	buf := bm.chooseUnpinnedBuffer()
	if buf == nil {
		return nil, ErrBufferPoolFull
	}

	// Flush buffer if dirty before reassigning
	if buf.IsDirty() {
		err := bm.flushBuffer(buf)
		if err != nil {
			return nil, err
		}
	}

	// Assign buffer to new block and read from disk
	buf.assignToBlock(block)
	_, err := bm.fm.Read(block, buf.Page())
	if err != nil {
		return nil, err
	}

	buf.Pin()
	bm.available--

	return buf, nil
}

// Unpin decrements the pin count of the buffer.
func (bm *BufferMgr) Unpin(buf *Buffer) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	buf.Unpin()
	if !buf.IsPinned() {
		bm.available++
	}
}

// FlushAll writes all dirty buffers to disk.
func (bm *BufferMgr) FlushAll(txnum int) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, buf := range bm.buffers {
		if buf.IsDirty() && buf.LogLSN() >= 0 {
			err := bm.flushBuffer(buf)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Available returns the number of available buffers.
func (bm *BufferMgr) Available() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.available
}

// findExistingBuffer searches for a buffer already assigned to the given block.
func (bm *BufferMgr) findExistingBuffer(block *file.BlockID) *Buffer {
	for _, buf := range bm.buffers {
		if buf.Block() != nil &&
			buf.Block().Filename == block.Filename &&
			buf.Block().Number == block.Number {
			return buf
		}
	}
	return nil
}

// chooseUnpinnedBuffer implements LRU replacement strategy.
func (bm *BufferMgr) chooseUnpinnedBuffer() *Buffer {
	// Simple implementation: find first unpinned buffer
	// TODO: Implement proper LRU algorithm
	for _, buf := range bm.buffers {
		if !buf.IsPinned() {
			return buf
		}
	}
	return nil
}

// flushBuffer writes a dirty buffer to disk.
func (bm *BufferMgr) flushBuffer(buf *Buffer) error {
	if buf.IsDirty() {
		// Ensure log records are flushed before data
		if buf.LogLSN() >= 0 {
			err := bm.lm.Flush()
			if err != nil {
				return err
			}
		}

		_, err := bm.fm.Write(buf.Block(), buf.Page())
		if err != nil {
			return err
		}
		buf.dirty = false
	}
	return nil
}

// PinTimeout attempts to pin a buffer with a timeout.
func (bm *BufferMgr) PinTimeout(block *file.BlockID, timeout time.Duration) (*Buffer, error) {
	start := time.Now()
	for time.Since(start) < timeout {
		buf, err := bm.Pin(block)
		if err == nil {
			return buf, nil
		}
		if err != ErrBufferPoolFull {
			return nil, err
		}

		// Wait a bit before retrying
		time.Sleep(10 * time.Millisecond)
	}
	return nil, ErrBufferTimeout
}
