package main

import (
	"fmt"
	"log"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	logmgr "github.com/inelpandzic/simpledb/log"
)

func main() {
	fmt.Println("SimpleDB - Starting...")

	// Initialize file manager
	dataDir := "simpledb_data"
	blockSize := 4096
	fm := file.NewFileMgr(dataDir, blockSize)
	defer fm.Close()

	// Initialize log manager
	lm := logmgr.NewLogMgr(fm, "simpledb.log")

	// Initialize buffer manager
	numBuffers := 8
	bm := buffer.NewBufferMgr(fm, lm, numBuffers)

	fmt.Printf("✅ File Manager initialized (block size: %d bytes)\n", blockSize)
	fmt.Printf("✅ Log Manager initialized\n")
	fmt.Printf("✅ Buffer Manager initialized (pool size: %d buffers)\n", numBuffers)

	// Test basic functionality
	testBlock := &file.BlockID{
		Filename: "test.db",
		Number:   0,
	}

	// First, create the file by writing an empty page
	emptyPage := file.NewPage(blockSize)
	_, err := fm.Write(testBlock, emptyPage)
	if err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}

	// Pin a buffer
	buf, err := bm.Pin(testBlock)
	if err != nil {
		log.Fatalf("Failed to pin buffer: %v", err)
	}

	// Write some test data
	testData := []byte("Hello SimpleDB!")
	_, err = buf.Page().Write(0, testData)
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}

	// Mark buffer as dirty (modified)
	buf.SetDirty(1)

	fmt.Printf("✅ Test data written to buffer\n")
	fmt.Printf("📊 Available buffers: %d\n", bm.Available())

	// Unpin the buffer
	bm.Unpin(buf)

	// Flush all buffers to disk
	err = bm.FlushAll(0)
	if err != nil {
		log.Fatalf("Failed to flush buffers: %v", err)
	}

	fmt.Printf("✅ All buffers flushed to disk\n")
	fmt.Printf("📊 Available buffers: %d\n", bm.Available())

	fmt.Println("\n🎯 Next Steps:")
	fmt.Println("   - ✅ Concurrency Manager (Complete!)")
	fmt.Println("   - Implement Recovery Manager")
	fmt.Println("   - Implement Record Management")
	fmt.Println("   - See PLAN.md for detailed roadmap")
	fmt.Println("\n🚀 For Concurrency Manager demo, run: go run examples/concurrency_demo.go")
}
