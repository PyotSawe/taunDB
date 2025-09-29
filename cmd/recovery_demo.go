package main

import (
	"fmt"
	"os"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
	"github.com/inelpandzic/simpledb/recovery"
)

func main() {
	// Clean up and create test directory
	testDir := "data/recovery_demo"
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)

	fmt.Println("=== SimpleDB Recovery Manager Demo ===\n")

	// Initialize managers
	fm := file.NewFileMgr(testDir, 1024)
	lm := log.NewLogMgr(fm, "recoverylog")

	bm := buffer.NewBufferMgr(fm, lm, 8)

	// Create recovery manager (will perform recovery on startup)
	fmt.Println("1. Creating Recovery Manager...")
	rm := recovery.NewRecoveryMgr(bm, lm, fm)
	fmt.Println("   ✓ Recovery Manager initialized\n")

	// Demonstrate transaction logging and recovery
	demoTransactionRecovery(rm, bm, fm)

	// Demonstrate checkpoint functionality
	demoCheckpoint(rm)

	fmt.Println("\n=== Recovery Demo Complete ===")
}

func demoTransactionRecovery(rm *recovery.RecoveryMgr, bm *buffer.BufferMgr, fm *file.FileMgr) {
	fmt.Println("2. Demonstrating Transaction Recovery...")

	// Create test file and block
	block := file.NewBlockID("testdata", 0)

	// Create the file first by writing an empty page
	emptyPage := file.NewPage(1024)
	initialData := "INITIAL DATA"
	copy(emptyPage.Contents()[0:], []byte(initialData))

	_, err := fm.Write(block, emptyPage)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ Initialized block with: '%s'\n", initialData)

	// Transaction 1: Complete transaction (will be committed)
	fmt.Println("\n   --- Transaction 1 (Committed) ---")
	lsn1, err := rm.WriteStartRecord(1)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ START record written (LSN: %d)\n", lsn1)

	// Simulate data update
	buf, err := bm.Pin(block)
	if err != nil {
		panic(err)
	}

	page := buf.Page()
	oldValue := make([]byte, len(initialData))
	copy(oldValue, page.Contents()[0:len(initialData)])
	newValue1 := "UPDATED BY TX1"

	lsn2, err := rm.WriteUpdateRecord(1, block, 0, oldValue, []byte(newValue1))
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ UPDATE record written (LSN: %d)\n", lsn2)

	// Apply the update
	copy(page.Contents()[0:], []byte(newValue1))
	buf.SetDirty(lsn2)
	bm.Unpin(buf)
	fmt.Printf("   ✓ Applied update: '%s'\n", newValue1)

	// Commit transaction
	lsn3, err := rm.WriteCommitRecord(1)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ COMMIT record written (LSN: %d)\n", lsn3)

	// Transaction 2: Incomplete transaction (will be aborted during recovery)
	fmt.Println("\n   --- Transaction 2 (Aborted) ---")
	lsn4, err := rm.WriteStartRecord(2)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ START record written (LSN: %d)\n", lsn4)

	// Simulate another update
	buf, err = bm.Pin(block)
	if err != nil {
		panic(err)
	}

	page = buf.Page()
	oldValue2 := make([]byte, len(newValue1))
	copy(oldValue2, page.Contents()[0:len(newValue1)])
	newValue2 := "UPDATED BY TX2"

	lsn5, err := rm.WriteUpdateRecord(2, block, 0, oldValue2, []byte(newValue2))
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ UPDATE record written (LSN: %d)\n", lsn5)

	// Apply the update
	copy(page.Contents()[0:], []byte(newValue2))
	buf.SetDirty(lsn5)
	bm.Unpin(buf)
	fmt.Printf("   ✓ Applied update: '%s'\n", newValue2)

	// NOTE: We intentionally don't commit transaction 2

	fmt.Println("\n   --- Simulating Crash and Recovery ---")

	// Read current state before recovery
	buf, err = bm.Pin(block)
	if err != nil {
		panic(err)
	}
	currentData := string(buf.Page().Contents()[0:len(newValue2)])
	bm.Unpin(buf)
	fmt.Printf("   Current data before recovery: '%s'\n", currentData)

	// Perform crash recovery
	fmt.Println("   Performing crash recovery...")
	err = rm.Recover()
	if err != nil {
		panic(err)
	}

	// Check data after recovery
	buf, err = bm.Pin(block)
	if err != nil {
		panic(err)
	}
	recoveredData := string(buf.Page().Contents()[0:len(newValue1)])
	bm.Unpin(buf)
	fmt.Printf("   ✓ Data after recovery: '%s'\n", recoveredData)

	if recoveredData == newValue1 {
		fmt.Println("   ✓ SUCCESS: Committed transaction preserved, uncommitted rolled back")
	} else {
		fmt.Println("   ✗ ERROR: Recovery did not work correctly")
	}
}

func demoCheckpoint(rm *recovery.RecoveryMgr) {
	fmt.Println("\n3. Demonstrating Checkpoint Functionality...")

	// Start some transactions
	activeTxs := []int{10, 20, 30}
	for _, txnum := range activeTxs {
		lsn, err := rm.WriteStartRecord(txnum)
		if err != nil {
			panic(err)
		}
		fmt.Printf("   ✓ Started transaction %d (LSN: %d)\n", txnum, lsn)
	}

	// Create checkpoint
	fmt.Println("   Creating checkpoint...")
	err := rm.Checkpoint(activeTxs)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✓ Checkpoint created with %d active transactions\n", len(activeTxs))

	// Commit some transactions
	for _, txnum := range activeTxs[:2] {
		lsn, err := rm.WriteCommitRecord(txnum)
		if err != nil {
			panic(err)
		}
		fmt.Printf("   ✓ Committed transaction %d (LSN: %d)\n", txnum, lsn)
	}

	fmt.Println("   ✓ Checkpoint demonstration complete")
}
