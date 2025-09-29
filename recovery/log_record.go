package recovery

import (
	"errors"
	"fmt"

	"github.com/inelpandzic/simpledb/file"
)

// LogRecordType represents different types of log records
type LogRecordType int

const (
	START LogRecordType = iota
	COMMIT
	ABORT
	UPDATE
	CHECKPOINT
)

var (
	ErrInvalidLogRecord = errors.New("invalid log record")
	ErrCorruptedLog     = errors.New("corrupted log record")
)

// LogRecord interface for all log record types
type LogRecord interface {
	Type() LogRecordType
	TxNum() int
	LSN() int
	String() string
	Undo(rm *RecoveryMgr) error
	Redo(rm *RecoveryMgr) error
}

// StartRecord represents a transaction start record
type StartRecord struct {
	txnum int
	lsn   int
}

func NewStartRecord(txnum int, lsn int) *StartRecord {
	return &StartRecord{txnum: txnum, lsn: lsn}
}

func (sr *StartRecord) Type() LogRecordType { return START }
func (sr *StartRecord) TxNum() int          { return sr.txnum }
func (sr *StartRecord) LSN() int            { return sr.lsn }
func (sr *StartRecord) String() string {
	return fmt.Sprintf("START tx=%d lsn=%d", sr.txnum, sr.lsn)
}
func (sr *StartRecord) Undo(rm *RecoveryMgr) error { return nil }
func (sr *StartRecord) Redo(rm *RecoveryMgr) error { return nil }

// CommitRecord represents a transaction commit record
type CommitRecord struct {
	txnum int
	lsn   int
}

func NewCommitRecord(txnum int, lsn int) *CommitRecord {
	return &CommitRecord{txnum: txnum, lsn: lsn}
}

func (cr *CommitRecord) Type() LogRecordType { return COMMIT }
func (cr *CommitRecord) TxNum() int          { return cr.txnum }
func (cr *CommitRecord) LSN() int            { return cr.lsn }
func (cr *CommitRecord) String() string {
	return fmt.Sprintf("COMMIT tx=%d lsn=%d", cr.txnum, cr.lsn)
}
func (cr *CommitRecord) Undo(rm *RecoveryMgr) error { return nil }
func (cr *CommitRecord) Redo(rm *RecoveryMgr) error { return nil }

// AbortRecord represents a transaction abort record
type AbortRecord struct {
	txnum int
	lsn   int
}

func NewAbortRecord(txnum int, lsn int) *AbortRecord {
	return &AbortRecord{txnum: txnum, lsn: lsn}
}

func (ar *AbortRecord) Type() LogRecordType { return ABORT }
func (ar *AbortRecord) TxNum() int          { return ar.txnum }
func (ar *AbortRecord) LSN() int            { return ar.lsn }
func (ar *AbortRecord) String() string {
	return fmt.Sprintf("ABORT tx=%d lsn=%d", ar.txnum, ar.lsn)
}
func (ar *AbortRecord) Undo(rm *RecoveryMgr) error { return nil }
func (ar *AbortRecord) Redo(rm *RecoveryMgr) error { return nil }

// UpdateRecord represents a data update record
type UpdateRecord struct {
	txnum    int
	lsn      int
	block    *file.BlockID
	offset   int
	oldValue []byte
	newValue []byte
}

func NewUpdateRecord(txnum int, lsn int, block *file.BlockID, offset int, oldValue, newValue []byte) *UpdateRecord {
	oldVal := make([]byte, len(oldValue))
	newVal := make([]byte, len(newValue))
	copy(oldVal, oldValue)
	copy(newVal, newValue)

	return &UpdateRecord{
		txnum:    txnum,
		lsn:      lsn,
		block:    block,
		offset:   offset,
		oldValue: oldVal,
		newValue: newVal,
	}
}

func (ur *UpdateRecord) Type() LogRecordType  { return UPDATE }
func (ur *UpdateRecord) TxNum() int           { return ur.txnum }
func (ur *UpdateRecord) LSN() int             { return ur.lsn }
func (ur *UpdateRecord) Block() *file.BlockID { return ur.block }
func (ur *UpdateRecord) Offset() int          { return ur.offset }
func (ur *UpdateRecord) OldValue() []byte     { return ur.oldValue }
func (ur *UpdateRecord) NewValue() []byte     { return ur.newValue }

func (ur *UpdateRecord) String() string {
	return fmt.Sprintf("UPDATE tx=%d lsn=%d block=%s offset=%d",
		ur.txnum, ur.lsn, ur.block.String(), ur.offset)
}

func (ur *UpdateRecord) Undo(rm *RecoveryMgr) error {
	// Pin the buffer and restore old value
	buf, err := rm.bm.Pin(ur.block)
	if err != nil {
		return err
	}
	defer rm.bm.Unpin(buf)

	// Write old value back to buffer
	page := buf.Page()
	copy(page.Contents()[ur.offset:], ur.oldValue)
	buf.SetDirty(ur.lsn)

	return nil
}

func (ur *UpdateRecord) Redo(rm *RecoveryMgr) error {
	// Pin the buffer and apply new value
	buf, err := rm.bm.Pin(ur.block)
	if err != nil {
		return err
	}
	defer rm.bm.Unpin(buf)

	// Write new value to buffer
	page := buf.Page()
	copy(page.Contents()[ur.offset:], ur.newValue)
	buf.SetDirty(ur.lsn)

	return nil
}

// CheckpointRecord represents a checkpoint record
type CheckpointRecord struct {
	lsn           int
	activeTxs     []int
	checkpointLSN int
}

func NewCheckpointRecord(lsn int, activeTxs []int) *CheckpointRecord {
	txs := make([]int, len(activeTxs))
	copy(txs, activeTxs)
	return &CheckpointRecord{
		lsn:           lsn,
		activeTxs:     txs,
		checkpointLSN: lsn,
	}
}

func (cp *CheckpointRecord) Type() LogRecordType { return CHECKPOINT }
func (cp *CheckpointRecord) TxNum() int          { return -1 } // No specific transaction
func (cp *CheckpointRecord) LSN() int            { return cp.lsn }
func (cp *CheckpointRecord) ActiveTxs() []int    { return cp.activeTxs }

func (cp *CheckpointRecord) String() string {
	return fmt.Sprintf("CHECKPOINT lsn=%d activeTxs=%v", cp.lsn, cp.activeTxs)
}

func (cp *CheckpointRecord) Undo(rm *RecoveryMgr) error { return nil }
func (cp *CheckpointRecord) Redo(rm *RecoveryMgr) error { return nil }
