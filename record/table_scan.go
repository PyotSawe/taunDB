package record

import (
	"fmt"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/concurrency"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/recovery"
)

// TableScan provides sequential access to records in a table.
type TableScan struct {
	tx         *concurrency.Transaction
	bm         *buffer.BufferMgr
	rm         *recovery.RecoveryMgr
	layout     *Layout
	tableName  string
	rp         *RecordPage
	currentRID *RID
	filename   string
}

// NewTableScan creates a new table scan for the specified table.
func NewTableScan(tx *concurrency.Transaction, tableName string, layout *Layout,
	bm *buffer.BufferMgr, rm *recovery.RecoveryMgr) *TableScan {

	filename := tableName + ".tbl"
	ts := &TableScan{
		tx:        tx,
		bm:        bm,
		rm:        rm,
		layout:    layout,
		tableName: tableName,
		filename:  filename,
	}

	// Try to move to the first block, if it doesn't exist, create it
	if !ts.moveToBlock(0) {
		// First block doesn't exist, create it
		if err := ts.createAndMoveToBlock(0); err != nil {
			// If we can't create block 0, leave ts.rp as nil
			// This will be handled by Insert() method
		}
	} else {
		// Block exists, position on first record
		ts.moveToNextRecord()
	}

	return ts
}

// Close closes the table scan and releases resources.
func (ts *TableScan) Close() {
	if ts.rp != nil && ts.rp.buffer != nil {
		ts.bm.Unpin(ts.rp.buffer)
	}
}

// Next moves to the next record in the table.
func (ts *TableScan) Next() bool {
	return ts.moveToNextRecord()
}

// HasData returns true if the scan is positioned on a valid record.
func (ts *TableScan) HasData() bool {
	return ts.currentRID != nil
}

// GetInt retrieves an integer value from the current record.
func (ts *TableScan) GetInt(fieldName string) (int, error) {
	if ts.currentRID == nil {
		return 0, fmt.Errorf("no current record")
	}
	return ts.rp.GetInt(ts.currentRID.Slot(), fieldName)
}

// GetString retrieves a string value from the current record.
func (ts *TableScan) GetString(fieldName string) (string, error) {
	if ts.currentRID == nil {
		return "", fmt.Errorf("no current record")
	}
	return ts.rp.GetString(ts.currentRID.Slot(), fieldName)
}

// SetInt sets an integer value in the current record.
func (ts *TableScan) SetInt(fieldName string, value int) error {
	if ts.currentRID == nil {
		return fmt.Errorf("no current record")
	}

	// Log the update for recovery
	oldBytes := make([]byte, 4)
	newBytes := make([]byte, 4)
	// Convert integers to bytes for logging
	// This is simplified - in a real implementation you'd have proper serialization

	offset := ts.layout.FieldOffset(ts.currentRID.Slot(), fieldName)
	_, err := ts.rm.WriteUpdateRecord(ts.tx.ID(), ts.currentRID.Block(), offset, oldBytes, newBytes)
	if err != nil {
		return err
	}

	return ts.rp.SetInt(ts.currentRID.Slot(), fieldName, value)
}

// SetString sets a string value in the current record.
func (ts *TableScan) SetString(fieldName string, value string) error {
	if ts.currentRID == nil {
		return fmt.Errorf("no current record")
	}

	// Log the update for recovery (simplified)
	offset := ts.layout.FieldOffset(ts.currentRID.Slot(), fieldName)
	oldBytes := []byte("old") // Simplified - should get actual old value
	newBytes := []byte(value)

	_, err := ts.rm.WriteUpdateRecord(ts.tx.ID(), ts.currentRID.Block(), offset, oldBytes, newBytes)
	if err != nil {
		return err
	}

	return ts.rp.SetString(ts.currentRID.Slot(), fieldName, value)
}

// Insert inserts a new record and positions the scan on it.
func (ts *TableScan) Insert() error {
	// If no current page, create the first block
	if ts.rp == nil {
		if err := ts.createAndMoveToBlock(0); err != nil {
			return fmt.Errorf("failed to create first block: %v", err)
		}
	}

	// Find an available slot in the current page or move to a new page
	slot := ts.rp.FindFirstAvailableSlot()
	if slot == -1 {
		// Current page is full, try to move to the next block
		if !ts.moveToNewBlock() {
			return fmt.Errorf("unable to find space for new record")
		}
		slot = ts.rp.FindFirstAvailableSlot()
		if slot == -1 {
			return fmt.Errorf("unable to find space for new record")
		}
	}

	// Mark the slot as in use
	err := ts.rp.SetInUse(slot, true)
	if err != nil {
		return err
	}

	// Update current position
	ts.currentRID = NewRID(ts.rp.Block(), slot)

	return nil
}

// Delete marks the current record as deleted.
func (ts *TableScan) Delete() error {
	if ts.currentRID == nil {
		return fmt.Errorf("no current record")
	}

	return ts.rp.SetInUse(ts.currentRID.Slot(), false)
}

// GetRID returns the RID of the current record.
func (ts *TableScan) GetRID() *RID {
	return ts.currentRID
}

// MoveToRID moves the scan to the specified RID.
func (ts *TableScan) MoveToRID(rid *RID) error {
	// Check if we need to switch to a different block
	if ts.rp == nil || ts.rp.Block().Filename != rid.Block().Filename ||
		ts.rp.Block().Number != rid.Block().Number {
		if !ts.moveToBlock(rid.Block().Number) {
			return fmt.Errorf("unable to move to block %d", rid.Block().Number)
		}
	}

	// Check if the slot is valid and in use
	if !ts.rp.IsValidSlot(rid.Slot()) {
		return fmt.Errorf("invalid slot: %d", rid.Slot())
	}

	if !ts.rp.IsInUse(rid.Slot()) {
		return fmt.Errorf("slot %d is not in use", rid.Slot())
	}

	ts.currentRID = rid
	return nil
}

// moveToBlock moves to the specified block number.
func (ts *TableScan) moveToBlock(blockNum int) bool {
	// Close current block if any
	if ts.rp != nil && ts.rp.buffer != nil {
		ts.bm.Unpin(ts.rp.buffer)
	}

	// Create new block ID
	block := file.NewBlockID(ts.filename, blockNum)

	// Try to pin the block
	buf, err := ts.bm.Pin(block)
	if err != nil {
		// Block doesn't exist - in a real implementation you'd create it
		// For now, just return false
		return false
	}

	// Create record page
	ts.rp = NewRecordPage(buf, ts.layout, block)
	ts.currentRID = nil

	return true
}

// createAndMoveToBlock creates a new block and moves to it.
func (ts *TableScan) createAndMoveToBlock(blockNum int) error {
	// Close current block if any
	if ts.rp != nil && ts.rp.buffer != nil {
		ts.bm.Unpin(ts.rp.buffer)
	}

	// Create new block ID
	block := file.NewBlockID(ts.filename, blockNum)

	// Pin a new buffer for this block
	buf, err := ts.bm.Pin(block)
	if err != nil {
		return fmt.Errorf("failed to pin block: %v", err)
	}

	// Create record page and format it
	ts.rp = NewRecordPage(buf, ts.layout, block)
	ts.rp.Format()
	ts.currentRID = nil

	return nil
}

// moveToNewBlock creates and moves to a new block.
func (ts *TableScan) moveToNewBlock() bool {
	// Find the next available block number
	nextBlockNum := 0
	if ts.rp != nil {
		nextBlockNum = ts.rp.Block().Number + 1
	}

	// Try to create and move to the new block
	if err := ts.createAndMoveToBlock(nextBlockNum); err != nil {
		return false
	}

	return true
}

// moveToNextRecord moves to the next valid record.
func (ts *TableScan) moveToNextRecord() bool {
	if ts.rp == nil {
		return false
	}

	// Start from the current slot or 0 if no current record
	startSlot := 0
	if ts.currentRID != nil {
		startSlot = ts.currentRID.Slot() + 1
	}

	maxSlots := ts.layout.SlotsPerBlock(ts.rp.buffer.Page().Size())

	// Look for the next in-use slot in the current block
	for slot := startSlot; slot < maxSlots; slot++ {
		if ts.rp.IsInUse(slot) {
			ts.currentRID = NewRID(ts.rp.Block(), slot)
			return true
		}
	}

	// No more records in current block, try next block
	if ts.moveToBlock(ts.rp.Block().Number + 1) {
		return ts.moveToNextRecord()
	}

	// No more records
	ts.currentRID = nil
	return false
}
