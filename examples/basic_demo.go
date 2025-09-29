package main

import (
	"fmt"
	"os"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/concurrency"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
	"github.com/inelpandzic/simpledb/recovery"
)

func main() {
	// Create data directory
	dataDir := "data/basic_demo"
	os.RemoveAll(dataDir)
	os.MkdirAll(dataDir, 0755)

	fmt.Println("=== SimpleDB Basic Demo ===")
	fmt.Println("Demonstrating: File Manager, Log Manager, Buffer Manager, Concurrency Manager, and Recovery Manager\n")

	// Initialize File Manager
	fmt.Println("1. Initializing File Manager...")
	fm := file.NewFileMgr(dataDir, 1024)
	fmt.Printf("   ✓ File Manager created (block size: %d bytes)\n", fm.BlockSize)

	// Initialize Log Manager
	fmt.Println("\n2. Initializing Log Manager...")
	lm := log.NewLogMgr(fm, "simpledb")
	fmt.Println("   ✓ Log Manager created")

	// Initialize Buffer Manager
	fmt.Println("\n3. Initializing Buffer Manager...")
	bm := buffer.NewBufferMgr(fm, lm, 8)
	fmt.Printf("   ✓ Buffer Manager created (pool size: %d buffers)\n", bm.Available())

	// Initialize Concurrency Manager
	fmt.Println("\n4. Initializing Concurrency Manager...")
	cm := concurrency.NewConcurrencyMgr(bm, 1000) // 1 second timeout
	fmt.Println("   ✓ Concurrency Manager created")

	// Initialize Recovery Manager
	fmt.Println("\n5. Initializing Recovery Manager...")
	rm := recovery.NewRecoveryMgr(bm, lm, fm)
	fmt.Println("   ✓ Recovery Manager created and recovery completed")

	// Demonstrate integrated transaction with recovery
	fmt.Println("\n6. Running integrated transaction demo...")
	demoIntegratedTransaction(fm, bm, cm, rm)

	fmt.Println("\n=== All components working successfully! ===")
}

func demoIntegratedTransaction(fm *file.FileMgr, bm *buffer.BufferMgr, cm *concurrency.ConcurrencyMgr, rm *recovery.RecoveryMgr) {
	// Start a transaction
	tx, err := cm.BeginTx()
	if err != nil {
		panic(err)
	}
	fmt.Printf("   Started transaction %d\n", tx.ID())

	// Log the transaction start
	lsn, err := rm.WriteStartRecord(tx.ID())
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ START record logged (LSN: %d)\n", lsn)

	// Create and lock a block
	block := file.NewBlockID("demo_table", 0)

	// Create the file first by writing an empty page
	emptyPage := file.NewPage(1024)
	_, err = fm.Write(block, emptyPage)
	if err != nil {
		panic(err)
	}

	// Pin buffer with concurrency control
	buf, err := cm.Pin(block, tx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ Block %s pinned and locked\n", block.String())

	// Write data with recovery logging
	page := buf.Page()
	oldData := make([]byte, 20)
	copy(oldData, page.Contents()[0:20])

	newData := []byte("Transaction Data    ") // 20 bytes

	// Log the update BEFORE making the change (Write-Ahead Logging)
	updateLSN, err := rm.WriteUpdateRecord(tx.ID(), block, 0, oldData, newData)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ UPDATE record logged (LSN: %d)\n", updateLSN)

	// Apply the change
	copy(page.Contents()[0:], newData)
	buf.SetDirty(updateLSN)
	fmt.Printf("   ✓ Data updated: '%s'\n", string(newData))

	// Unpin buffer (still locked)
	cm.Unpin(buf)

	// Commit the transaction
	err = cm.CommitTx(tx)
	if err != nil {
		panic(err)
	}

	// Log the commit
	commitLSN, err := rm.WriteCommitRecord(tx.ID())
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ Transaction committed (LSN: %d)\n", commitLSN)

	// Verify data persistence
	buf2, err := bm.Pin(block)
	if err != nil {
		panic(err)
	}
	defer bm.Unpin(buf2)

	verifyData := string(buf2.Page().Contents()[0:20])
	fmt.Printf("   ✓ Data verification: '%s'\n", verifyData)

	if verifyData == string(newData) {
		fmt.Println("   ✓ SUCCESS: All components working together!")
	} else {
		fmt.Println("   ✗ ERROR: Data integrity issue")
	}
}
