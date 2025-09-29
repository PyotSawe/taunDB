package recovery

import (
	"fmt"
	"sync"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
)

// RecoveryMgr handles crash recovery using write-ahead logging
type RecoveryMgr struct {
	bm                *buffer.BufferMgr
	lm                *log.LogMgr
	fm                *file.FileMgr
	lastCheckpointLSN int
	mu                sync.RWMutex
}

// NewRecoveryMgr creates a new recovery manager
func NewRecoveryMgr(bm *buffer.BufferMgr, lm *log.LogMgr, fm *file.FileMgr) *RecoveryMgr {
	rm := &RecoveryMgr{
		bm:                bm,
		lm:                lm,
		fm:                fm,
		lastCheckpointLSN: -1,
	}

	// Perform recovery on startup
	err := rm.Recover()
	if err != nil {
		fmt.Printf("Recovery failed on startup: %v\n", err)
		// Don't panic - just log the error for now
	}

	return rm
}

// WriteStartRecord writes a START record to the log
func (rm *RecoveryMgr) WriteStartRecord(txnum int) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record := log.NewRecordBuilder()
	record.WriteInt(int(START))
	record.WriteInt(txnum)

	lsn, err := rm.lm.Append(record.GetData())
	if err != nil {
		return -1, err
	}

	return lsn, nil
}

// WriteCommitRecord writes a COMMIT record to the log
func (rm *RecoveryMgr) WriteCommitRecord(txnum int) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record := log.NewRecordBuilder()
	record.WriteInt(int(COMMIT))
	record.WriteInt(txnum)

	lsn, err := rm.lm.Append(record.GetData())
	if err != nil {
		return -1, err
	}

	// Force log to disk for commit records
	err = rm.lm.Flush()
	if err != nil {
		return -1, err
	}

	return lsn, nil
}

// WriteAbortRecord writes an ABORT record to the log
func (rm *RecoveryMgr) WriteAbortRecord(txnum int) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record := log.NewRecordBuilder()
	record.WriteInt(int(ABORT))
	record.WriteInt(txnum)

	lsn, err := rm.lm.Append(record.GetData())
	if err != nil {
		return -1, err
	}

	return lsn, nil
}

// WriteUpdateRecord writes an UPDATE record to the log
func (rm *RecoveryMgr) WriteUpdateRecord(txnum int, block *file.BlockID, offset int, oldValue, newValue []byte) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record := log.NewRecordBuilder()
	record.WriteInt(int(UPDATE))
	record.WriteInt(txnum)
	record.WriteString(block.Filename)
	record.WriteInt(block.Number)
	record.WriteInt(offset)
	record.WriteInt(len(oldValue))
	record.WriteBytes(oldValue)
	record.WriteInt(len(newValue))
	record.WriteBytes(newValue)

	lsn, err := rm.lm.Append(record.GetData())
	if err != nil {
		return -1, err
	}

	return lsn, nil
}

// WriteCheckpointRecord writes a CHECKPOINT record to the log
func (rm *RecoveryMgr) WriteCheckpointRecord(activeTxs []int) (int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record := log.NewRecordBuilder()
	record.WriteInt(int(CHECKPOINT))
	record.WriteInt(len(activeTxs))
	for _, txnum := range activeTxs {
		record.WriteInt(txnum)
	}

	lsn, err := rm.lm.Append(record.GetData())
	if err != nil {
		return -1, err
	}

	// Force log to disk for checkpoint records
	err = rm.lm.Flush()
	if err != nil {
		return -1, err
	}

	rm.lastCheckpointLSN = lsn
	return lsn, nil
}

// Recover performs crash recovery using ARIES algorithm principles
func (rm *RecoveryMgr) Recover() error {
	fmt.Println("Starting crash recovery...")

	// Phase 1: Analysis - scan log to identify transactions and dirty pages
	committedTxs, abortedTxs, err := rm.doAnalysis()
	if err != nil {
		return fmt.Errorf("analysis phase failed: %w", err)
	}

	// Phase 2: Redo - replay all operations to restore database state
	err = rm.doRedo()
	if err != nil {
		return fmt.Errorf("redo phase failed: %w", err)
	}

	// Phase 3: Undo - rollback uncommitted transactions
	err = rm.doUndo(committedTxs, abortedTxs)
	if err != nil {
		return fmt.Errorf("undo phase failed: %w", err)
	}

	fmt.Println("Crash recovery completed successfully")
	return nil
}

// doAnalysis scans the log to identify transaction states
func (rm *RecoveryMgr) doAnalysis() (map[int]bool, map[int]bool, error) {
	fmt.Println("Recovery Phase 1: Analysis")

	committedTxs := make(map[int]bool)
	abortedTxs := make(map[int]bool)
	activeTxs := make(map[int]bool)

	iter, err := rm.lm.Iterator()
	if err != nil {
		// If log doesn't exist yet, that's fine - empty database
		fmt.Println("No log file found - fresh database startup")
		return committedTxs, abortedTxs, nil
	}

	for iter.HasNext() {
		record, err := iter.Next()
		if err != nil {
			return nil, nil, err
		}

		logRecord, err := rm.parseLogRecord(record.Data)
		if err != nil {
			continue // Skip corrupted records
		}

		switch logRecord.Type() {
		case START:
			activeTxs[logRecord.TxNum()] = true
		case COMMIT:
			committedTxs[logRecord.TxNum()] = true
			delete(activeTxs, logRecord.TxNum())
		case ABORT:
			abortedTxs[logRecord.TxNum()] = true
			delete(activeTxs, logRecord.TxNum())
		case CHECKPOINT:
			if cp, ok := logRecord.(*CheckpointRecord); ok {
				rm.lastCheckpointLSN = cp.LSN()
			}
		}
	}

	// Any remaining active transactions are considered aborted
	for txnum := range activeTxs {
		abortedTxs[txnum] = true
	}

	fmt.Printf("Analysis complete: %d committed, %d aborted transactions\n",
		len(committedTxs), len(abortedTxs))

	return committedTxs, abortedTxs, nil
}

// doRedo replays all operations from the log
func (rm *RecoveryMgr) doRedo() error {
	fmt.Println("Recovery Phase 2: Redo")

	iter, err := rm.lm.Iterator()
	if err != nil {
		// No log file - nothing to redo
		fmt.Println("No log file for redo - skipping")
		return nil
	}

	redoCount := 0
	for iter.HasNext() {
		record, err := iter.Next()
		if err != nil {
			return err
		}

		logRecord, err := rm.parseLogRecord(record.Data)
		if err != nil {
			continue // Skip corrupted records
		}

		if logRecord.Type() == UPDATE {
			err = logRecord.Redo(rm)
			if err != nil {
				fmt.Printf("Warning: Failed to redo operation: %v\n", err)
				// Continue with recovery - don't fail completely
				continue
			}
			redoCount++
		}
	}

	fmt.Printf("Redo complete: %d operations replayed\n", redoCount)
	return nil
}

// doUndo rolls back uncommitted transactions
func (rm *RecoveryMgr) doUndo(committedTxs, abortedTxs map[int]bool) error {
	fmt.Println("Recovery Phase 3: Undo")

	// Collect all log records in reverse order
	var allRecords []LogRecord
	iter, err := rm.lm.Iterator()
	if err != nil {
		// No log file - nothing to undo
		fmt.Println("No log file for undo - skipping")
		return nil
	}

	for iter.HasNext() {
		record, err := iter.Next()
		if err != nil {
			return err
		}

		logRecord, err := rm.parseLogRecord(record.Data)
		if err != nil {
			continue
		}
		allRecords = append(allRecords, logRecord)
	}

	// Process in reverse order (most recent first)
	undoCount := 0
	for i := len(allRecords) - 1; i >= 0; i-- {
		record := allRecords[i]

		// Only undo operations from uncommitted transactions
		if record.Type() == UPDATE && !committedTxs[record.TxNum()] {
			err = record.Undo(rm)
			if err != nil {
				fmt.Printf("Warning: Failed to undo operation: %v\n", err)
				// Continue with recovery
				continue
			}
			undoCount++
		}
	}

	fmt.Printf("Undo complete: %d operations rolled back\n", undoCount)
	return nil
}

// parseLogRecord parses log data into a LogRecord
func (rm *RecoveryMgr) parseLogRecord(data []byte) (LogRecord, error) {
	if len(data) < 4 {
		return nil, ErrCorruptedLog
	}

	record := log.NewRecordFromData(data)
	recordType := LogRecordType(record.ReadInt())

	switch recordType {
	case START:
		txnum := record.ReadInt()
		return NewStartRecord(txnum, -1), nil

	case COMMIT:
		txnum := record.ReadInt()
		return NewCommitRecord(txnum, -1), nil

	case ABORT:
		txnum := record.ReadInt()
		return NewAbortRecord(txnum, -1), nil

	case UPDATE:
		txnum := record.ReadInt()
		filename := record.ReadString()
		blockNum := record.ReadInt()
		offset := record.ReadInt()
		oldLen := record.ReadInt()
		oldValue := record.ReadBytes(oldLen)
		newLen := record.ReadInt()
		newValue := record.ReadBytes(newLen)

		block := file.NewBlockID(filename, blockNum)
		return NewUpdateRecord(txnum, -1, block, offset, oldValue, newValue), nil

	case CHECKPOINT:
		numTxs := record.ReadInt()
		activeTxs := make([]int, numTxs)
		for i := 0; i < numTxs; i++ {
			activeTxs[i] = record.ReadInt()
		}
		return NewCheckpointRecord(-1, activeTxs), nil

	default:
		return nil, ErrInvalidLogRecord
	}
}

// Checkpoint creates a checkpoint record
func (rm *RecoveryMgr) Checkpoint(activeTxs []int) error {
	// Flush all buffers first
	err := rm.bm.FlushAll(-1)
	if err != nil {
		return err
	}

	// Write checkpoint record
	_, err = rm.WriteCheckpointRecord(activeTxs)
	return err
}
