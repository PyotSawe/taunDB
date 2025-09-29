package concurrency

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
)

func TestLockManager(t *testing.T) {
	lm := NewLockMgr()

	block1 := &file.BlockID{Filename: "test1.db", Number: 0}

	// Test shared locks
	err := lm.SLock(block1, 1)
	if err != nil {
		t.Errorf("Failed to acquire shared lock: %v", err)
	}

	err = lm.SLock(block1, 2)
	if err != nil {
		t.Errorf("Failed to acquire second shared lock: %v", err)
	}

	// Test exclusive lock conflict
	lm.SetLockTimeout(100 * time.Millisecond)
	err = lm.XLock(block1, 3)
	if err != ErrTimeout {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	// Release locks and try exclusive lock
	lm.Unlock(1)
	lm.Unlock(2)

	err = lm.XLock(block1, 3)
	if err != nil {
		t.Errorf("Failed to acquire exclusive lock after release: %v", err)
	}

	// Test shared lock conflict with exclusive
	err = lm.SLock(block1, 4)
	if err != ErrTimeout {
		t.Errorf("Expected timeout error for shared lock conflict, got: %v", err)
	}

	lm.Unlock(3)
}

func TestTransactionTable(t *testing.T) {
	tt := NewTxTable()

	// Test transaction creation
	tx1 := tt.BeginTx()
	tx2 := tt.BeginTx()

	if tx1 >= tx2 {
		t.Errorf("Transaction numbers should be increasing: tx1=%d, tx2=%d", tx1, tx2)
	}

	// Test transaction states
	if !tt.IsActive(tx1) {
		t.Error("Transaction should be active")
	}

	// Test commit
	err := tt.CommitTx(tx1)
	if err != nil {
		t.Errorf("Failed to commit transaction: %v", err)
	}

	if tt.IsActive(tx1) {
		t.Error("Transaction should not be active after commit")
	}

	// Test abort
	err = tt.AbortTx(tx2)
	if err != nil {
		t.Errorf("Failed to abort transaction: %v", err)
	}

	if tt.IsActive(tx2) {
		t.Error("Transaction should not be active after abort")
	}

	// Test invalid operations
	err = tt.CommitTx(tx1) // Already committed
	if err != ErrInvalidTransactionState {
		t.Errorf("Expected invalid state error, got: %v", err)
	}

	err = tt.CommitTx(999) // Non-existent
	if err != ErrTransactionNotFound {
		t.Errorf("Expected not found error, got: %v", err)
	}
}

func TestConcurrencyManager(t *testing.T) {
	dataDir := "testdata_concurrency"
	blockSize := 32
	numBuffers := 3

	// Create test directory
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	fm := file.NewFileMgr(dataDir, blockSize)
	lm := log.NewLogMgr(fm, "testlog")
	bm := buffer.NewBufferMgr(fm, lm, numBuffers)
	cm := NewConcurrencyMgr(bm)

	t.Cleanup(func() {
		fm.Close()
		os.RemoveAll(dataDir)
	})

	// Set short timeout for testing
	cm.SetLockTimeout(100 * time.Millisecond)

	// Test transaction lifecycle
	tx1 := cm.BeginTransaction()
	tx2 := cm.BeginTransaction()

	block1 := &file.BlockID{Filename: "test.db", Number: 0}

	// Create the test file
	emptyPage := file.NewPage(blockSize)
	fm.Write(block1, emptyPage)

	// Test shared lock and pin
	buf1, err := cm.Pin(block1, tx1)
	if err != nil {
		t.Errorf("Failed to pin with shared lock: %v", err)
	}

	// Test concurrent shared lock
	buf2, err := cm.Pin(block1, tx2)
	if err != nil {
		t.Errorf("Failed to acquire concurrent shared lock: %v", err)
	}

	cm.Unpin(buf1)
	cm.Unpin(buf2)

	// First commit tx2 to release its shared lock
	cm.CommitTransaction(tx2)

	// Test exclusive lock
	buf3, err := cm.PinForUpdate(block1, tx1)
	if err != nil {
		t.Errorf("Failed to pin for update: %v", err)
	}

	// Test conflicting exclusive lock (should timeout)
	tx3 := cm.BeginTransaction()
	_, err = cm.PinForUpdate(block1, tx3)
	if err == nil {
		t.Error("Expected error for conflicting exclusive lock")
	}

	cm.Unpin(buf3)
	cm.AbortTransaction(tx3)

	// Test transaction commit
	err = cm.CommitTransaction(tx1)
	if err != nil {
		t.Errorf("Failed to commit transaction: %v", err)
	}

	// Now a new transaction should be able to acquire exclusive lock
	tx4 := cm.BeginTransaction()
	buf4, err := cm.PinForUpdate(block1, tx4)
	if err != nil {
		t.Errorf("Failed to acquire exclusive lock after commit: %v", err)
	}

	cm.Unpin(buf4)
	cm.AbortTransaction(tx4)
}

func TestConcurrentTransactions(t *testing.T) {
	dataDir := "testdata_concurrent"
	blockSize := 32
	numBuffers := 5

	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	fm := file.NewFileMgr(dataDir, blockSize)
	lm := log.NewLogMgr(fm, "testlog")
	bm := buffer.NewBufferMgr(fm, lm, numBuffers)
	cm := NewConcurrencyMgr(bm)

	t.Cleanup(func() {
		fm.Close()
		os.RemoveAll(dataDir)
	})

	block1 := &file.BlockID{Filename: "concurrent.db", Number: 0}

	// Create test file
	emptyPage := file.NewPage(blockSize)
	fm.Write(block1, emptyPage)

	// Set a short timeout for this test
	cm.SetLockTimeout(50 * time.Millisecond)

	const numTxs = 5
	const numOperations = 10

	var wg sync.WaitGroup
	results := make(chan error, numTxs*numOperations)

	// Start multiple transactions concurrently
	for i := 0; i < numTxs; i++ {
		wg.Add(1)
		go func(txIndex int) {
			defer wg.Done()

			tx := cm.BeginTransaction()

			for j := 0; j < numOperations; j++ {
				// Randomly choose shared or exclusive lock
				if (txIndex+j)%2 == 0 {
					buf, err := cm.Pin(block1, tx)
					if err != nil {
						results <- err
						continue
					}
					time.Sleep(time.Millisecond) // Simulate work
					cm.Unpin(buf)
				} else {
					buf, err := cm.PinForUpdate(block1, tx)
					if err != nil {
						// Timeouts and lock conflicts are expected in concurrent scenarios
						if err != ErrTimeout && !strings.Contains(err.Error(), "timeout") {
							results <- err
						}
						continue
					}
					time.Sleep(time.Millisecond) // Simulate work
					cm.Unpin(buf)
				}
			}

			// Randomly commit or abort
			if txIndex%2 == 0 {
				err := cm.CommitTransaction(tx)
				if err != nil {
					results <- err
				}
			} else {
				err := cm.AbortTransaction(tx)
				if err != nil {
					results <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(results)

	// Check for unexpected errors
	for err := range results {
		t.Errorf("Unexpected error in concurrent test: %v", err)
	}

	// Verify all transactions are finished
	activeTxs := cm.GetActiveTransactions()
	if len(activeTxs) != 0 {
		t.Errorf("Expected no active transactions, found: %v", activeTxs)
	}
}

func TestDeadlockDetection(t *testing.T) {
	lm := NewLockMgr()
	lm.SetLockTimeout(50 * time.Millisecond)

	block1 := &file.BlockID{Filename: "deadlock1.db", Number: 0}
	block2 := &file.BlockID{Filename: "deadlock2.db", Number: 0}

	// Transaction 1 locks block1
	err := lm.XLock(block1, 1)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Transaction 2 locks block2
	err = lm.XLock(block2, 2)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)

	// Transaction 1 tries to lock block2 (will wait)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := lm.XLock(block2, 1)
		results <- err
	}()

	// Transaction 2 tries to lock block1 (should detect deadlock or timeout)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // Ensure tx1 starts waiting first
		err := lm.XLock(block1, 2)
		results <- err
	}()

	wg.Wait()
	close(results)

	errorCount := 0
	for err := range results {
		if err != nil {
			errorCount++
			// Should be either timeout or deadlock error
			if err != ErrTimeout && err != ErrDeadlock {
				t.Errorf("Unexpected error type: %v", err)
			}
		}
	}

	if errorCount != 2 {
		t.Errorf("Expected 2 errors (deadlock/timeout), got %d", errorCount)
	}

	lm.Unlock(1)
	lm.Unlock(2)
}

func TestLockManagerStats(t *testing.T) {
	dataDir := "testdata_stats"
	blockSize := 32
	numBuffers := 3

	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	fm := file.NewFileMgr(dataDir, blockSize)
	lm := log.NewLogMgr(fm, "testlog")
	bm := buffer.NewBufferMgr(fm, lm, numBuffers)
	cm := NewConcurrencyMgr(bm)

	t.Cleanup(func() {
		fm.Close()
		os.RemoveAll(dataDir)
	})

	// Test initial stats
	stats := cm.GetStats()
	if stats.ActiveTransactions != 0 {
		t.Errorf("Expected 0 active transactions, got %d", stats.ActiveTransactions)
	}

	// Start some transactions
	tx1 := cm.BeginTransaction()
	tx2 := cm.BeginTransaction()

	stats = cm.GetStats()
	if stats.ActiveTransactions != 2 {
		t.Errorf("Expected 2 active transactions, got %d", stats.ActiveTransactions)
	}

	// Commit one transaction
	cm.CommitTransaction(tx1)

	stats = cm.GetStats()
	if stats.ActiveTransactions != 1 {
		t.Errorf("Expected 1 active transaction, got %d", stats.ActiveTransactions)
	}

	// Abort the other
	cm.AbortTransaction(tx2)

	stats = cm.GetStats()
	if stats.ActiveTransactions != 0 {
		t.Errorf("Expected 0 active transactions, got %d", stats.ActiveTransactions)
	}
}
