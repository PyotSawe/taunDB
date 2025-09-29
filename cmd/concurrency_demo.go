package main

import (
	"fmt"
	"log"
	"time"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/concurrency"
	"github.com/inelpandzic/simpledb/file"
	logmgr "github.com/inelpandzic/simpledb/log"
)

func main() {
	fmt.Println("SimpleDB Concurrency Manager Demo")
	fmt.Println("==================================")

	// Initialize components
	dataDir := "simpledb_data"
	blockSize := 4096
	numBuffers := 8

	fm := file.NewFileMgr(dataDir, blockSize)
	defer fm.Close()

	lm := logmgr.NewLogMgr(fm, "simpledb.log")
	bm := buffer.NewBufferMgr(fm, lm, numBuffers)
	cm := concurrency.NewConcurrencyMgr(bm)

	fmt.Printf("✅ Concurrency Manager initialized\n")
	fmt.Printf("📊 Initial stats: %+v\n", cm.GetStats())

	// Create test blocks
	block1 := &file.BlockID{Filename: "accounts.db", Number: 0}
	block2 := &file.BlockID{Filename: "accounts.db", Number: 1}

	// Create test files
	emptyPage := file.NewPage(blockSize)
	fm.Write(block1, emptyPage)
	fm.Write(block2, emptyPage)

	// Demo 1: Basic transaction with shared locks
	fmt.Println("\n🔄 Demo 1: Basic Transaction with Shared Locks")
	tx1 := cm.BeginTransaction()
	fmt.Printf("Started transaction %d\n", tx1)

	buf1, err := cm.Pin(block1, tx1)
	if err != nil {
		log.Fatalf("Failed to pin block: %v", err)
	}
	fmt.Printf("✅ Transaction %d acquired shared lock on block1\n", tx1)

	// Simulate reading data
	data := []byte("Account Balance: $1000")
	buf1.Page().Write(0, data)
	fmt.Printf("📖 Transaction %d read/modified data\n", tx1)

	cm.Unpin(buf1)
	fmt.Printf("📌 Transaction %d unpinned buffer\n", tx1)

	err = cm.CommitTransaction(tx1)
	if err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}
	fmt.Printf("✅ Transaction %d committed\n", tx1)

	// Demo 2: Concurrent shared locks
	fmt.Println("\n🔄 Demo 2: Concurrent Shared Locks")
	tx2 := cm.BeginTransaction()
	tx3 := cm.BeginTransaction()
	fmt.Printf("Started transactions %d and %d\n", tx2, tx3)

	buf2, err := cm.Pin(block1, tx2)
	if err != nil {
		log.Fatalf("Failed to pin block for tx2: %v", err)
	}

	buf3, err := cm.Pin(block1, tx3)
	if err != nil {
		log.Fatalf("Failed to pin block for tx3: %v", err)
	}

	fmt.Printf("✅ Both transactions acquired shared locks on block1\n")
	fmt.Printf("📊 Block1 locks: %v\n", cm.GetBlockLocks(block1))

	cm.Unpin(buf2)
	cm.Unpin(buf3)
	cm.CommitTransaction(tx2)
	cm.CommitTransaction(tx3)
	fmt.Printf("✅ Both transactions committed\n")

	// Demo 3: Exclusive lock conflict
	fmt.Println("\n🔄 Demo 3: Exclusive Lock Conflict")
	cm.SetLockTimeout(2 * time.Second)

	tx4 := cm.BeginTransaction()
	tx5 := cm.BeginTransaction()
	fmt.Printf("Started transactions %d and %d\n", tx4, tx5)

	buf4, err := cm.PinForUpdate(block2, tx4)
	if err != nil {
		log.Fatalf("Failed to pin for update: %v", err)
	}
	fmt.Printf("✅ Transaction %d acquired exclusive lock on block2\n", tx4)

	// Try to acquire conflicting exclusive lock
	fmt.Printf("🔒 Transaction %d trying to acquire exclusive lock on block2...\n", tx5)
	start := time.Now()
	_, err = cm.PinForUpdate(block2, tx5)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ Transaction %d failed to acquire lock after %v: %v\n", tx5, duration, err)
	} else {
		fmt.Printf("✅ Transaction %d acquired lock unexpectedly\n", tx5)
	}

	cm.Unpin(buf4)
	cm.CommitTransaction(tx4)
	cm.AbortTransaction(tx5)
	fmt.Printf("✅ Resolved conflict - tx4 committed, tx5 aborted\n")

	// Demo 4: Lock after release
	fmt.Println("\n🔄 Demo 4: Lock Acquisition After Release")
	tx6 := cm.BeginTransaction()
	tx7 := cm.BeginTransaction()

	buf6, err := cm.PinForUpdate(block2, tx6)
	if err != nil {
		log.Fatalf("Failed to pin for update: %v", err)
	}
	fmt.Printf("✅ Transaction %d acquired exclusive lock on block2\n", tx6)

	// Release the lock
	cm.Unpin(buf6)
	cm.CommitTransaction(tx6)
	fmt.Printf("🔓 Transaction %d released lock\n", tx6)

	// Now tx7 should be able to acquire the lock
	buf7, err := cm.PinForUpdate(block2, tx7)
	if err != nil {
		log.Fatalf("Failed to pin after release: %v", err)
	}
	fmt.Printf("✅ Transaction %d acquired exclusive lock after tx6 released it\n", tx7)

	cm.Unpin(buf7)
	cm.CommitTransaction(tx7)

	// Final stats
	fmt.Println("\n📊 Final Statistics")
	stats := cm.GetStats()
	fmt.Printf("Active Transactions: %d\n", stats.ActiveTransactions)
	fmt.Printf("Total Transactions: %d\n", stats.TotalTransactions)
	fmt.Printf("Available Buffers: %d\n", stats.AvailableBuffers)

	activeTxs := cm.GetActiveTransactions()
	fmt.Printf("Active Transaction IDs: %v\n", activeTxs)

	// Cleanup old transactions
	cleaned := cm.CleanupOldTransactions(1 * time.Minute)
	fmt.Printf("Cleaned up %d old transactions\n", cleaned)

	fmt.Println("\n🎯 Concurrency Manager Demo Complete!")
	fmt.Println("✅ Demonstrated:")
	fmt.Println("   - Shared lock acquisition")
	fmt.Println("   - Concurrent shared locks") 
	fmt.Println("   - Exclusive lock conflicts")
	fmt.Println("   - Lock timeout handling")
	fmt.Println("   - Transaction lifecycle management")
}