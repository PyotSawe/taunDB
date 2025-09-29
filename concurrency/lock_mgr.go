package concurrency

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inelpandzic/simpledb/file"
)

var (
	ErrLockConflict = errors.New("lock conflict detected")
	ErrDeadlock     = errors.New("deadlock detected")
	ErrTimeout      = errors.New("lock acquisition timeout")
)

// LockType represents the type of lock.
type LockType int

const (
	SharedLock    LockType = iota // S-lock
	ExclusiveLock                 // X-lock
)

func (lt LockType) String() string {
	switch lt {
	case SharedLock:
		return "S"
	case ExclusiveLock:
		return "X"
	default:
		return "Unknown"
	}
}

// Lock represents a lock on a block.
type Lock struct {
	Block    *file.BlockID
	Type     LockType
	TxNum    int
	Acquired time.Time
}

// String returns a string representation of the lock.
func (l *Lock) String() string {
	return fmt.Sprintf("Lock{Block: %s:%d, Type: %s, TxNum: %d}",
		l.Block.Filename, l.Block.Number, l.Type, l.TxNum)
}

// WaitingTx represents a transaction waiting for a lock.
type WaitingTx struct {
	TxNum     int
	LockType  LockType
	Timestamp time.Time
	Done      chan bool
}

// LockMgr manages locks for concurrent access to blocks.
type LockMgr struct {
	locks       map[string][]*Lock      // Maps block ID to list of locks
	waitingTxs  map[string][]*WaitingTx // Maps block ID to waiting transactions
	lockTimeout time.Duration           // Maximum time to wait for a lock
	mu          sync.RWMutex
}

// NewLockMgr creates a new lock manager.
func NewLockMgr() *LockMgr {
	return &LockMgr{
		locks:       make(map[string][]*Lock),
		waitingTxs:  make(map[string][]*WaitingTx),
		lockTimeout: 10 * time.Second, // Default timeout
	}
}

// SetLockTimeout sets the maximum time to wait for a lock.
func (lm *LockMgr) SetLockTimeout(timeout time.Duration) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.lockTimeout = timeout
}

// SLock acquires a shared lock on the specified block.
func (lm *LockMgr) SLock(block *file.BlockID, txnum int) error {
	return lm.sLockWithTimeout(block, txnum, lm.lockTimeout)
}

// XLock acquires an exclusive lock on the specified block.
func (lm *LockMgr) XLock(block *file.BlockID, txnum int) error {
	return lm.xLockWithTimeout(block, txnum, lm.lockTimeout)
}

// SLockWithTimeout acquires a shared lock with a specific timeout.
func (lm *LockMgr) sLockWithTimeout(block *file.BlockID, txnum int, timeout time.Duration) error {
	blockKey := blockToKey(block)

	lm.mu.Lock()

	// Check if transaction already holds a lock on this block
	if lm.hasLockByTx(blockKey, txnum) {
		lm.mu.Unlock()
		return nil // Already has lock
	}

	// Check if we can acquire the lock immediately
	if !lm.hasExclusiveLock(blockKey, txnum) {
		// Can acquire immediately
		lock := &Lock{
			Block:    block,
			Type:     SharedLock,
			TxNum:    txnum,
			Acquired: time.Now(),
		}
		lm.locks[blockKey] = append(lm.locks[blockKey], lock)
		lm.mu.Unlock()
		return nil
	}

	// Need to wait - check for potential deadlock
	if lm.wouldCauseDeadlock(blockKey, txnum) {
		lm.mu.Unlock()
		return ErrDeadlock
	}

	// Add to waiting list
	waitingTx := &WaitingTx{
		TxNum:     txnum,
		LockType:  SharedLock,
		Timestamp: time.Now(),
		Done:      make(chan bool, 1),
	}
	lm.waitingTxs[blockKey] = append(lm.waitingTxs[blockKey], waitingTx)
	lm.mu.Unlock()

	// Wait for lock with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case <-waitingTx.Done:
		return nil
	case <-ctx.Done():
		// Remove from waiting list
		lm.mu.Lock()
		lm.removeWaitingTx(blockKey, txnum)
		lm.mu.Unlock()
		return ErrTimeout
	}
}

// XLockWithTimeout acquires an exclusive lock with a specific timeout.
func (lm *LockMgr) xLockWithTimeout(block *file.BlockID, txnum int, timeout time.Duration) error {
	blockKey := blockToKey(block)

	lm.mu.Lock()

	// Check if transaction already holds an exclusive lock on this block
	if lm.hasExclusiveLockByTx(blockKey, txnum) {
		lm.mu.Unlock()
		return nil // Already has exclusive lock
	}

	// Check if we can acquire the lock immediately
	if !lm.hasConflictingLock(blockKey, txnum) {
		// Can acquire immediately
		lock := &Lock{
			Block:    block,
			Type:     ExclusiveLock,
			TxNum:    txnum,
			Acquired: time.Now(),
		}
		lm.locks[blockKey] = append(lm.locks[blockKey], lock)
		lm.mu.Unlock()
		return nil
	}

	// Need to wait - check for potential deadlock
	if lm.wouldCauseDeadlock(blockKey, txnum) {
		lm.mu.Unlock()
		return ErrDeadlock
	}

	// Add to waiting list
	waitingTx := &WaitingTx{
		TxNum:     txnum,
		LockType:  ExclusiveLock,
		Timestamp: time.Now(),
		Done:      make(chan bool, 1),
	}
	lm.waitingTxs[blockKey] = append(lm.waitingTxs[blockKey], waitingTx)
	lm.mu.Unlock()

	// Wait for lock with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case <-waitingTx.Done:
		return nil
	case <-ctx.Done():
		// Remove from waiting list
		lm.mu.Lock()
		lm.removeWaitingTx(blockKey, txnum)
		lm.mu.Unlock()
		return ErrTimeout
	}
}

// Unlock releases all locks held by the specified transaction.
func (lm *LockMgr) Unlock(txnum int) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	releasedBlocks := make([]string, 0)

	// Remove all locks held by this transaction
	for blockKey, lockList := range lm.locks {
		filteredLocks := make([]*Lock, 0, len(lockList))
		for _, lock := range lockList {
			if lock.TxNum != txnum {
				filteredLocks = append(filteredLocks, lock)
			}
		}

		if len(filteredLocks) != len(lockList) {
			releasedBlocks = append(releasedBlocks, blockKey)
		}

		if len(filteredLocks) == 0 {
			delete(lm.locks, blockKey)
		} else {
			lm.locks[blockKey] = filteredLocks
		}
	}

	// Notify waiting transactions for released blocks
	for _, blockKey := range releasedBlocks {
		lm.notifyWaitingTransactions(blockKey)
	}
}

// GetLocksForTx returns all locks held by a specific transaction.
func (lm *LockMgr) GetLocksForTx(txnum int) []*Lock {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var txLocks []*Lock
	for _, lockList := range lm.locks {
		for _, lock := range lockList {
			if lock.TxNum == txnum {
				txLocks = append(txLocks, lock)
			}
		}
	}
	return txLocks
}

// GetBlockLocks returns all locks on a specific block.
func (lm *LockMgr) GetBlockLocks(block *file.BlockID) []*Lock {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	blockKey := blockToKey(block)
	locks := lm.locks[blockKey]
	result := make([]*Lock, len(locks))
	copy(result, locks)
	return result
}

// Helper methods

// hasLockByTx checks if a transaction already holds any lock on the block.
func (lm *LockMgr) hasLockByTx(blockKey string, txnum int) bool {
	locks := lm.locks[blockKey]
	for _, lock := range locks {
		if lock.TxNum == txnum {
			return true
		}
	}
	return false
}

// hasExclusiveLockByTx checks if a transaction holds an exclusive lock on the block.
func (lm *LockMgr) hasExclusiveLockByTx(blockKey string, txnum int) bool {
	locks := lm.locks[blockKey]
	for _, lock := range locks {
		if lock.TxNum == txnum && lock.Type == ExclusiveLock {
			return true
		}
	}
	return false
}

// hasExclusiveLock checks if there's an exclusive lock by another transaction.
func (lm *LockMgr) hasExclusiveLock(blockKey string, txnum int) bool {
	locks := lm.locks[blockKey]
	for _, lock := range locks {
		if lock.Type == ExclusiveLock && lock.TxNum != txnum {
			return true
		}
	}
	return false
}

// hasConflictingLock checks if there are any conflicting locks.
func (lm *LockMgr) hasConflictingLock(blockKey string, txnum int) bool {
	locks := lm.locks[blockKey]
	for _, lock := range locks {
		if lock.TxNum != txnum {
			return true
		}
	}
	return false
}

// wouldCauseDeadlock performs simple deadlock detection.
func (lm *LockMgr) wouldCauseDeadlock(blockKey string, txnum int) bool {
	// Simple heuristic: if transaction is already waiting for another lock,
	// and other transactions are waiting for this transaction's locks,
	// there might be a deadlock

	// Check if this transaction is already waiting somewhere
	for otherBlockKey, waitingList := range lm.waitingTxs {
		if otherBlockKey == blockKey {
			continue
		}
		for _, waiting := range waitingList {
			if waiting.TxNum == txnum {
				// This transaction is already waiting elsewhere
				// Check if any transaction holding current block is waiting for our transaction
				currentLocks := lm.locks[blockKey]
				for _, lock := range currentLocks {
					if lm.isTransactionWaitingFor(lock.TxNum, txnum) {
						return true
					}
				}
			}
		}
	}
	return false
}

// isTransactionWaitingFor checks if tx1 is waiting for tx2.
func (lm *LockMgr) isTransactionWaitingFor(tx1, tx2 int) bool {
	myLocks := make([]string, 0)

	// Find blocks locked by tx2
	for blockKey, lockList := range lm.locks {
		for _, lock := range lockList {
			if lock.TxNum == tx2 {
				myLocks = append(myLocks, blockKey)
				break
			}
		}
	}

	// Check if tx1 is waiting for any of those blocks
	for _, blockKey := range myLocks {
		waitingList := lm.waitingTxs[blockKey]
		for _, waiting := range waitingList {
			if waiting.TxNum == tx1 {
				return true
			}
		}
	}
	return false
}

// removeWaitingTx removes a transaction from the waiting list.
func (lm *LockMgr) removeWaitingTx(blockKey string, txnum int) {
	waitingList := lm.waitingTxs[blockKey]
	filteredList := make([]*WaitingTx, 0, len(waitingList))

	for _, waiting := range waitingList {
		if waiting.TxNum != txnum {
			filteredList = append(filteredList, waiting)
		}
	}

	if len(filteredList) == 0 {
		delete(lm.waitingTxs, blockKey)
	} else {
		lm.waitingTxs[blockKey] = filteredList
	}
}

// notifyWaitingTransactions notifies waiting transactions that locks may be available.
func (lm *LockMgr) notifyWaitingTransactions(blockKey string) {
	waitingList := lm.waitingTxs[blockKey]
	if len(waitingList) == 0 {
		return
	}

	// Process waiting transactions in FIFO order
	newWaitingList := make([]*WaitingTx, 0)

	for _, waiting := range waitingList {
		canGrant := false

		if waiting.LockType == SharedLock {
			canGrant = !lm.hasExclusiveLock(blockKey, waiting.TxNum)
		} else { // ExclusiveLock
			canGrant = !lm.hasConflictingLock(blockKey, waiting.TxNum)
		}

		if canGrant {
			// Parse block from blockKey for proper assignment
			// This is a simplified approach - in production you'd want better parsing
			parts := strings.Split(blockKey, ":")
			blockNum := 0
			if len(parts) == 2 {
				if num, err := strconv.Atoi(parts[1]); err == nil {
					blockNum = num
				}
			}

			// Grant the lock
			lock := &Lock{
				Block: &file.BlockID{
					Filename: parts[0],
					Number:   blockNum,
				},
				Type:     waiting.LockType,
				TxNum:    waiting.TxNum,
				Acquired: time.Now(),
			}
			lm.locks[blockKey] = append(lm.locks[blockKey], lock)

			// Notify the waiting transaction
			select {
			case waiting.Done <- true:
			default:
			}
		} else {
			newWaitingList = append(newWaitingList, waiting)
		}
	}

	if len(newWaitingList) == 0 {
		delete(lm.waitingTxs, blockKey)
	} else {
		lm.waitingTxs[blockKey] = newWaitingList
	}
}

// blockToKey converts a block ID to a string key.
func blockToKey(block *file.BlockID) string {
	return block.Filename + ":" + strconv.Itoa(block.Number)
}
