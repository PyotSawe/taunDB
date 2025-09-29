package concurrency

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrTransactionNotFound     = errors.New("transaction not found")
	ErrInvalidTransactionState = errors.New("invalid transaction state")
)

// TxStatus represents the status of a transaction.
type TxStatus int

const (
	TxActive TxStatus = iota
	TxCommitted
	TxAborted
	TxPrepared // For two-phase commit (future extension)
)

func (ts TxStatus) String() string {
	switch ts {
	case TxActive:
		return "Active"
	case TxCommitted:
		return "Committed"
	case TxAborted:
		return "Aborted"
	case TxPrepared:
		return "Prepared"
	default:
		return "Unknown"
	}
}

// TxInfo holds information about a transaction.
type TxInfo struct {
	TxNum     int
	Status    TxStatus
	StartTime time.Time
	EndTime   *time.Time
	LockCount int
}

// TxTable manages active transactions and their states.
type TxTable struct {
	transactions map[int]*TxInfo
	nextTxNum    int64
	mu           sync.RWMutex
}

// NewTxTable creates a new transaction table.
func NewTxTable() *TxTable {
	return &TxTable{
		transactions: make(map[int]*TxInfo),
		nextTxNum:    1,
	}
}

// BeginTx starts a new transaction and returns its transaction number.
func (tt *TxTable) BeginTx() int {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	txnum := int(atomic.AddInt64(&tt.nextTxNum, 1))

	txInfo := &TxInfo{
		TxNum:     txnum,
		Status:    TxActive,
		StartTime: time.Now(),
		LockCount: 0,
	}

	tt.transactions[txnum] = txInfo
	return txnum
}

// CommitTx marks a transaction as committed.
func (tt *TxTable) CommitTx(txnum int) error {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	txInfo, exists := tt.transactions[txnum]
	if !exists {
		return ErrTransactionNotFound
	}

	if txInfo.Status != TxActive {
		return ErrInvalidTransactionState
	}

	now := time.Now()
	txInfo.Status = TxCommitted
	txInfo.EndTime = &now

	return nil
}

// AbortTx marks a transaction as aborted.
func (tt *TxTable) AbortTx(txnum int) error {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	txInfo, exists := tt.transactions[txnum]
	if !exists {
		return ErrTransactionNotFound
	}

	if txInfo.Status != TxActive {
		return ErrInvalidTransactionState
	}

	now := time.Now()
	txInfo.Status = TxAborted
	txInfo.EndTime = &now

	return nil
}

// GetTxInfo returns information about a transaction.
func (tt *TxTable) GetTxInfo(txnum int) (*TxInfo, error) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	txInfo, exists := tt.transactions[txnum]
	if !exists {
		return nil, ErrTransactionNotFound
	}

	// Return a copy to avoid race conditions
	infoCopy := *txInfo
	return &infoCopy, nil
}

// IsActive returns true if the transaction is active.
func (tt *TxTable) IsActive(txnum int) bool {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	txInfo, exists := tt.transactions[txnum]
	return exists && txInfo.Status == TxActive
}

// GetActiveTxs returns a list of all active transaction numbers.
func (tt *TxTable) GetActiveTxs() []int {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	var activeTxs []int
	for txnum, txInfo := range tt.transactions {
		if txInfo.Status == TxActive {
			activeTxs = append(activeTxs, txnum)
		}
	}
	return activeTxs
}

// GetAllTxs returns information about all transactions.
func (tt *TxTable) GetAllTxs() map[int]*TxInfo {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	result := make(map[int]*TxInfo)
	for txnum, txInfo := range tt.transactions {
		infoCopy := *txInfo
		result[txnum] = &infoCopy
	}
	return result
}

// IncrementLockCount increments the lock count for a transaction.
func (tt *TxTable) IncrementLockCount(txnum int) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if txInfo, exists := tt.transactions[txnum]; exists {
		txInfo.LockCount++
	}
}

// DecrementLockCount decrements the lock count for a transaction.
func (tt *TxTable) DecrementLockCount(txnum int) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if txInfo, exists := tt.transactions[txnum]; exists && txInfo.LockCount > 0 {
		txInfo.LockCount--
	}
}

// CleanupFinishedTxs removes finished transactions older than the specified duration.
func (tt *TxTable) CleanupFinishedTxs(olderThan time.Duration) int {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removedCount := 0

	for txnum, txInfo := range tt.transactions {
		if txInfo.Status != TxActive && txInfo.EndTime != nil && txInfo.EndTime.Before(cutoff) {
			delete(tt.transactions, txnum)
			removedCount++
		}
	}

	return removedCount
}

// GetTransactionCount returns the total number of transactions in the table.
func (tt *TxTable) GetTransactionCount() int {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	return len(tt.transactions)
}

// GetActiveTransactionCount returns the number of active transactions.
func (tt *TxTable) GetActiveTransactionCount() int {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	count := 0
	for _, txInfo := range tt.transactions {
		if txInfo.Status == TxActive {
			count++
		}
	}
	return count
}
