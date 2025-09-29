package file

import "fmt"

// BlockID represents a disk block for a corresponding file.
type BlockID struct {
	Filename string
	Number   int
}

// NewBlockID creates a new BlockID
func NewBlockID(filename string, number int) *BlockID {
	return &BlockID{
		Filename: filename,
		Number:   number,
	}
}

// String returns string representation of the block
func (b *BlockID) String() string {
	return fmt.Sprintf("%s:%d", b.Filename, b.Number)
}
