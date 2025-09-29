package record

import (
	"fmt"

	"github.com/inelpandzic/simpledb/file"
)

// RID (Record Identifier) uniquely identifies a record by its block and slot.
type RID struct {
	block *file.BlockID
	slot  int
}

// NewRID creates a new record identifier.
func NewRID(block *file.BlockID, slot int) *RID {
	return &RID{
		block: block,
		slot:  slot,
	}
}

// Block returns the block ID containing this record.
func (r *RID) Block() *file.BlockID {
	return r.block
}

// Slot returns the slot number of this record within the block.
func (r *RID) Slot() int {
	return r.slot
}

// String returns a string representation of the RID.
func (r *RID) String() string {
	return fmt.Sprintf("RID[%s, slot=%d]", r.block.String(), r.slot)
}

// Equals checks if two RIDs are equal.
func (r *RID) Equals(other *RID) bool {
	if other == nil {
		return false
	}
	return r.block.Filename == other.block.Filename &&
		r.block.Number == other.block.Number &&
		r.slot == other.slot
}
