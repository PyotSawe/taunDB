package buffer

import (
	"os"
	"testing"

	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
)

func TestBufferManager(t *testing.T) {
	dataDir := "testdata"
	blockSize := 32
	numBuffers := 3

	// Create the test directory
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	fm := file.NewFileMgr(dataDir, blockSize)
	lm := log.NewLogMgr(fm, "testlog")
	bm := NewBufferMgr(fm, lm, numBuffers)

	t.Cleanup(func() {
		fm.Close()
		os.RemoveAll(dataDir)
	})

	// Test basic pin/unpin
	block1 := &file.BlockID{Filename: "testfile", Number: 0}

	// Create the file first
	emptyPage := file.NewPage(blockSize)
	_, err = fm.Write(block1, emptyPage)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	buf1, err := bm.Pin(block1)
	if err != nil {
		t.Fatalf("Pin failed: %v", err)
	}

	if bm.Available() != numBuffers-1 {
		t.Errorf("Available = %d, want %d", bm.Available(), numBuffers-1)
	}

	// Test unpinning
	bm.Unpin(buf1)
	if bm.Available() != numBuffers {
		t.Errorf("Available after unpin = %d, want %d", bm.Available(), numBuffers)
	}

	// Test pinning same block twice
	buf1a, _ := bm.Pin(block1)
	buf1b, _ := bm.Pin(block1)

	if buf1a != buf1b {
		t.Error("Same block should return same buffer")
	}

	if bm.Available() != numBuffers-1 {
		t.Errorf("Available with double pin = %d, want %d", bm.Available(), numBuffers-1)
	}

	bm.Unpin(buf1a)
	bm.Unpin(buf1b)
}

func TestBufferPoolExhaustion(t *testing.T) {
	dataDir := "testdata2"
	blockSize := 32
	numBuffers := 2

	// Create the test directory
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	fm := file.NewFileMgr(dataDir, blockSize)
	lm := log.NewLogMgr(fm, "testlog")
	bm := NewBufferMgr(fm, lm, numBuffers)

	t.Cleanup(func() {
		fm.Close()
		os.RemoveAll(dataDir)
	})

	// Pin all buffers
	block1 := &file.BlockID{Filename: "testfile1", Number: 0}
	block2 := &file.BlockID{Filename: "testfile2", Number: 0}
	block3 := &file.BlockID{Filename: "testfile3", Number: 0}

	// Create the files first
	emptyPage := file.NewPage(blockSize)
	fm.Write(block1, emptyPage)
	fm.Write(block2, emptyPage)
	fm.Write(block3, emptyPage)

	buf1, _ := bm.Pin(block1)
	buf2, _ := bm.Pin(block2)

	// This should fail as all buffers are pinned
	_, err = bm.Pin(block3)
	if err != ErrBufferPoolFull {
		t.Errorf("Expected buffer pool full error, got: %v", err)
	}

	// Unpin one buffer and try again
	bm.Unpin(buf1)
	buf3, err := bm.Pin(block3)
	if err != nil {
		t.Errorf("Pin should succeed after unpin: %v", err)
	}

	bm.Unpin(buf2)
	if buf3 != nil {
		bm.Unpin(buf3)
	}
}

func TestBuffer(t *testing.T) {
	blockSize := 32
	buf := NewBuffer(blockSize)

	// Test initial state
	if buf.IsPinned() {
		t.Error("New buffer should not be pinned")
	}

	if buf.IsDirty() {
		t.Error("New buffer should not be dirty")
	}

	// Test pinning
	buf.Pin()
	if !buf.IsPinned() {
		t.Error("Buffer should be pinned after Pin()")
	}

	buf.Pin()
	buf.Unpin()
	if !buf.IsPinned() {
		t.Error("Buffer should still be pinned after one unpin")
	}

	buf.Unpin()
	if buf.IsPinned() {
		t.Error("Buffer should not be pinned after all unpins")
	}

	// Test dirty flag
	buf.SetDirty(123)
	if !buf.IsDirty() {
		t.Error("Buffer should be dirty after SetDirty()")
	}

	if buf.LogLSN() != 123 {
		t.Errorf("LogLSN = %d, want 123", buf.LogLSN())
	}
}
