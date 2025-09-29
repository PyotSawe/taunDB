package concurrency

import (
	"fmt"
	"sync"
	"time"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
)

// Transaction represents a database transaction
type Transaction struct {
	id        int
	cm        *ConcurrencyMgr
	startTime time.Time
}

// ID returns the transaction ID
func (tx *Transaction) ID() int {
	return tx.id
}

// ConcurrencyMgr coordinates transaction management and locking.
type ConcurrencyMgr struct {
	lockMgr *LockMgr
	txTable *TxTable
	bufMgr  *buffer.BufferMgr
	mu      sync.RWMutex
}

// NewConcurrencyMgr creates a new concurrency manager.
func NewConcurrencyMgr(bufMgr *buffer.BufferMgr, timeoutMs ...int) *ConcurrencyMgr {
	cm := &ConcurrencyMgr{
		lockMgr: NewLockMgr(),
		txTable: NewTxTable(),
		bufMgr:  bufMgr,
	}

	// Set lock timeout (default to 1000ms if not specified)
	timeout := 1000
	if len(timeoutMs) > 0 {
		timeout = timeoutMs[0]
	}
	cm.lockMgr.SetLockTimeout(time.Duration(timeout) * time.Millisecond)

	return cm
}

// BeginTx starts a new transaction
func (cm *ConcurrencyMgr) BeginTx() (*Transaction, error) {
	txID := cm.txTable.BeginTx()

	tx := &Transaction{
		id:        txID,
		cm:        cm,
		startTime: time.Now(),
	}

	return tx, nil
}

// BeginTransaction starts a new transaction (alias for BeginTx)
func (cm *ConcurrencyMgr) BeginTransaction() *Transaction {
	tx, _ := cm.BeginTx()
	return tx
}

// CommitTx commits a transaction and releases all its locks
func (cm *ConcurrencyMgr) CommitTx(tx *Transaction) error {
	// First commit the transaction
	err := cm.txTable.CommitTx(tx.id)
	if err != nil {
		return fmt.Errorf("failed to commit transaction %d: %w", tx.id, err)
	}

	// Release all locks held by this transaction
	cm.lockMgr.Unlock(tx.id)

	return nil
}

// CommitTransaction commits a transaction (alias for CommitTx)
func (cm *ConcurrencyMgr) CommitTransaction(tx *Transaction) error {
	return cm.CommitTx(tx)
}

// AbortTx aborts a transaction and releases all its locks
func (cm *ConcurrencyMgr) AbortTx(tx *Transaction) error {
	// First abort the transaction
	err := cm.txTable.AbortTx(tx.id)
	if err != nil {
		return fmt.Errorf("failed to abort transaction %d: %w", tx.id, err)
	}

	// Release all locks held by this transaction
	cm.lockMgr.Unlock(tx.id)

	return nil
}

// AbortTransaction aborts a transaction (alias for AbortTx)
func (cm *ConcurrencyMgr) AbortTransaction(tx *Transaction) error {
	return cm.AbortTx(tx)
}

// Pin pins a buffer and acquires a shared lock on it
func (cm *ConcurrencyMgr) Pin(block *file.BlockID, tx *Transaction) (*buffer.Buffer, error) {
	// First acquire the shared lock
	err := cm.lockMgr.SLock(block, tx.id)
	if err != nil {
		return nil, err
	}

	// Then pin the buffer
	buf, err := cm.bufMgr.Pin(block)
	if err != nil {
		return nil, fmt.Errorf("failed to pin buffer after acquiring lock: %w", err)
	}

	return buf, nil
}

// PinForUpdate pins a buffer and acquires an exclusive lock on it
func (cm *ConcurrencyMgr) PinForUpdate(block *file.BlockID, tx *Transaction) (*buffer.Buffer, error) {
	// First acquire the exclusive lock
	err := cm.lockMgr.XLock(block, tx.id)
	if err != nil {
		return nil, err
	}

	// Then pin the buffer
	buf, err := cm.bufMgr.Pin(block)
	if err != nil {
		return nil, fmt.Errorf("failed to pin buffer after acquiring exclusive lock: %w", err)
	}

	return buf, nil
}

// Unpin unpins a buffer (but keeps the lock)
func (cm *ConcurrencyMgr) Unpin(buf *buffer.Buffer) {
	cm.bufMgr.Unpin(buf)
}

// SLock acquires a shared lock on a block for a transaction
func (cm *ConcurrencyMgr) SLock(block *file.BlockID, txnum int) error {
	// Check if transaction is active
	if !cm.txTable.IsActive(txnum) {
		return fmt.Errorf("transaction %d is not active", txnum)
	}

	// Acquire the shared lock
	err := cm.lockMgr.SLock(block, txnum)
	if err != nil {
		return fmt.Errorf("failed to acquire shared lock on %s:%d for tx %d: %w",
			block.Filename, block.Number, txnum, err)
	}

	// Increment lock count
	cm.txTable.IncrementLockCount(txnum)

	return nil
}

// XLock acquires an exclusive lock on a block for a transaction
func (cm *ConcurrencyMgr) XLock(block *file.BlockID, txnum int) error {
	// Check if transaction is active
	if !cm.txTable.IsActive(txnum) {
		return fmt.Errorf("transaction %d is not active", txnum)
	}

	// Acquire the exclusive lock
	err := cm.lockMgr.XLock(block, txnum)
	if err != nil {
		return fmt.Errorf("failed to acquire exclusive lock on %s:%d for tx %d: %w",
			block.Filename, block.Number, txnum, err)
	}

	// Increment lock count
	cm.txTable.IncrementLockCount(txnum)

	return nil
}

// GetTransactionInfo returns information about a transaction
func (cm *ConcurrencyMgr) GetTransactionInfo(txnum int) (*TxInfo, error) {
	return cm.txTable.GetTxInfo(txnum)
}

// GetTransactionLocks returns all locks held by a transaction
func (cm *ConcurrencyMgr) GetTransactionLocks(txnum int) []*Lock {
	return cm.lockMgr.GetLocksForTx(txnum)
}

// GetBlockLocks returns all locks on a specific block
func (cm *ConcurrencyMgr) GetBlockLocks(block *file.BlockID) []*Lock {
	return cm.lockMgr.GetBlockLocks(block)
}

// GetActiveTransactions returns a list of all active transaction numbers
func (cm *ConcurrencyMgr) GetActiveTransactions() []int {
	return cm.txTable.GetActiveTxs()
}

// SetLockTimeout sets the timeout for lock acquisition
func (cm *ConcurrencyMgr) SetLockTimeout(timeout time.Duration) {
	cm.lockMgr.SetLockTimeout(timeout)
}

// CleanupOldTransactions removes finished transactions older than the specified duration
func (cm *ConcurrencyMgr) CleanupOldTransactions(olderThan time.Duration) int {
	return cm.txTable.CleanupFinishedTxs(olderThan)
}

// GetStats returns concurrency manager statistics
func (cm *ConcurrencyMgr) GetStats() ConcurrencyStats {
	return ConcurrencyStats{
		ActiveTransactions: cm.txTable.GetActiveTransactionCount(),
		TotalTransactions:  cm.txTable.GetTransactionCount(),
		AvailableBuffers:   cm.bufMgr.Available(),
	}
}

// ConcurrencyStats holds statistics about the concurrency manager
type ConcurrencyStats struct {
	ActiveTransactions int
	TotalTransactions  int
	AvailableBuffers   int
}
