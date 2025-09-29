package main

import (
	"fmt"
	"os"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
	"github.com/inelpandzic/simpledb/recovery"
)

// Transaction represents a database transaction
type Transaction struct {
	id       int
	rm       *recovery.RecoveryMgr
	bm       *buffer.BufferMgr
	startLSN int
}

// NewTransaction creates and starts a new transaction
func NewTransaction(id int, rm *recovery.RecoveryMgr, bm *buffer.BufferMgr) (*Transaction, error) {
	tx := &Transaction{
		id: id,
		rm: rm,
		bm: bm,
	}

	lsn, err := rm.WriteStartRecord(id)
	if err != nil {
		return nil, err
	}

	tx.startLSN = lsn
	fmt.Printf("Transaction %d started (LSN: %d)\n", id, lsn)
	return tx, nil
}

// UpdateData performs a data update with proper logging
func (tx *Transaction) UpdateData(block *file.BlockID, offset int, newData []byte) error {
	// Pin the buffer
	buf, err := tx.bm.Pin(block)
	if err != nil {
		return err
	}
	defer tx.bm.Unpin(buf)

	// Read old value
	page := buf.Page()
	oldData := make([]byte, len(newData))
	copy(oldData, page.Contents()[offset:offset+len(newData)])

	// Write update log record BEFORE modifying data (WAL)
	lsn, err := tx.rm.WriteUpdateRecord(tx.id, block, offset, oldData, newData)
	if err != nil {
		return err
	}

	// Apply the change
	copy(page.Contents()[offset:], newData)
	buf.SetDirty(lsn)

	fmt.Printf("Transaction %d updated block %s at offset %d (LSN: %d)\n",
		tx.id, block.String(), offset, lsn)
	return nil
}

// Commit commits the transaction
func (tx *Transaction) Commit() error {
	lsn, err := tx.rm.WriteCommitRecord(tx.id)
	if err != nil {
		return err
	}

	fmt.Printf("Transaction %d committed (LSN: %d)\n", tx.id, lsn)
	return nil
}

// Abort aborts the transaction
func (tx *Transaction) Abort() error {
	lsn, err := tx.rm.WriteAbortRecord(tx.id)
	if err != nil {
		return err
	}

	fmt.Printf("Transaction %d aborted (LSN: %d)\n", tx.id, lsn)
	return nil
}

func main() {
	fmt.Println("=== Recovery Manager Integration Example ===\n")

	// Setup
	testDir := "data/recovery_integration"
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)

	fm := file.NewFileMgr(testDir, 1024)
	lm := log.NewLogMgr(fm, "integrationlog")

	bm := buffer.NewBufferMgr(fm, lm, 8)
	rm := recovery.NewRecoveryMgr(bm, lm, fm)

	// Create test blocks
	block1 := file.NewBlockID("table1", 0)
	block2 := file.NewBlockID("table2", 0)

	// Initialize data
	initializeData(fm, bm, block1, "Initial Data 1")
	initializeData(fm, bm, block2, "Initial Data 2")

	// Demonstrate multiple concurrent transactions
	fmt.Println("1. Running multiple transactions...")

	// Transaction 1: Updates both blocks and commits
	tx1, err := NewTransaction(100, rm, bm)
	if err != nil {
		panic(err)
	}

	err = tx1.UpdateData(block1, 0, []byte("Updated by TX100"))
	if err != nil {
		panic(err)
	}

	err = tx1.UpdateData(block2, 0, []byte("Modified by TX100"))
	if err != nil {
		panic(err)
	}

	err = tx1.Commit()
	if err != nil {
		panic(err)
	}

	// Transaction 2: Updates data but aborts
	tx2, err := NewTransaction(200, rm, bm)
	if err != nil {
		panic(err)
	}

	err = tx2.UpdateData(block1, 0, []byte("Changed by TX200"))
	if err != nil {
		panic(err)
	}

	err = tx2.Abort()
	if err != nil {
		panic(err)
	}

	// Transaction 3: Updates data but doesn't commit (simulates crash)
	tx3, err := NewTransaction(300, rm, bm)
	if err != nil {
		panic(err)
	}

	err = tx3.UpdateData(block2, 0, []byte("Temp change TX300"))
	if err != nil {
		panic(err)
	}
	// Note: No commit or abort - simulates crash

	fmt.Println("\n2. Current data state:")
	printBlockData(bm, block1, "Block1")
	printBlockData(bm, block2, "Block2")

	// Perform checkpoint
	fmt.Println("\n3. Creating checkpoint...")
	activeTxs := []int{300} // Transaction 3 is still active
	err = rm.Checkpoint(activeTxs)
	if err != nil {
		panic(err)
	}

	// Simulate crash and recovery
	fmt.Println("\n4. Simulating crash and recovery...")
	err = rm.Recover()
	if err != nil {
		panic(err)
	}

	fmt.Println("\n5. Data state after recovery:")
	printBlockData(bm, block1, "Block1")
	printBlockData(bm, block2, "Block2")

	fmt.Println("\n=== Integration Example Complete ===")
	fmt.Println("Expected results:")
	fmt.Println("- Block1: Should show TX100 changes (committed)")
	fmt.Println("- Block2: Should show TX100 changes, TX300 rolled back")
}

func initializeData(fm *file.FileMgr, bm *buffer.BufferMgr, block *file.BlockID, data string) {
	// Create the file first by writing an empty page
	emptyPage := file.NewPage(1024)
	copy(emptyPage.Contents()[0:], []byte(data))

	// Write the page to create the file
	_, err := fm.Write(block, emptyPage)
	if err != nil {
		panic(err)
	}
}

func printBlockData(bm *buffer.BufferMgr, block *file.BlockID, name string) {
	buf, err := bm.Pin(block)
	if err != nil {
		panic(err)
	}
	defer bm.Unpin(buf)

	page := buf.Page()
	data := string(page.Contents()[0:20]) // Read first 20 bytes
	fmt.Printf("   %s: '%s'\n", name, data)
}
