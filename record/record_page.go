package record

import (
	"encoding/binary"
	"fmt"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
)

// RecordPage manages records within a single page/block.
type RecordPage struct {
	buffer *buffer.Buffer
	layout *Layout
	block  *file.BlockID
}

// NewRecordPage creates a new record page from a buffer and layout.
func NewRecordPage(buffer *buffer.Buffer, layout *Layout, block *file.BlockID) *RecordPage {
	return &RecordPage{
		buffer: buffer,
		layout: layout,
		block:  block,
	}
}

// GetInt retrieves an integer value from the specified field and slot.
func (rp *RecordPage) GetInt(slot int, fieldName string) (int, error) {
	if !rp.layout.Schema().HasField(fieldName) {
		return 0, ErrInvalidField
	}

	fieldType, _ := rp.layout.Schema().Type(fieldName)
	if fieldType != IntegerType {
		return 0, fmt.Errorf("field %s is not an integer", fieldName)
	}

	offset := rp.layout.FieldOffset(slot, fieldName)
	page := rp.buffer.Page()

	// Read 4 bytes for integer
	intBytes := make([]byte, 4)
	page.Read(offset, intBytes)

	return int(binary.LittleEndian.Uint32(intBytes)), nil
}

// SetInt sets an integer value in the specified field and slot.
func (rp *RecordPage) SetInt(slot int, fieldName string, value int) error {
	if !rp.layout.Schema().HasField(fieldName) {
		return ErrInvalidField
	}

	fieldType, _ := rp.layout.Schema().Type(fieldName)
	if fieldType != IntegerType {
		return fmt.Errorf("field %s is not an integer", fieldName)
	}

	offset := rp.layout.FieldOffset(slot, fieldName)
	page := rp.buffer.Page()

	// Write 4 bytes for integer
	intBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(intBytes, uint32(value))

	_, err := page.Write(offset, intBytes)
	if err != nil {
		return err
	}

	rp.buffer.SetDirty(-1) // Mark buffer as dirty
	return nil
}

// GetString retrieves a string value from the specified field and slot.
func (rp *RecordPage) GetString(slot int, fieldName string) (string, error) {
	if !rp.layout.Schema().HasField(fieldName) {
		return "", ErrInvalidField
	}

	fieldType, _ := rp.layout.Schema().Type(fieldName)
	if fieldType != VarcharType {
		return "", fmt.Errorf("field %s is not a string", fieldName)
	}

	offset := rp.layout.FieldOffset(slot, fieldName)
	page := rp.buffer.Page()

	// Read 4 bytes for string length
	lengthBytes := make([]byte, 4)
	page.Read(offset, lengthBytes)
	length := int(binary.LittleEndian.Uint32(lengthBytes))

	// Read the actual string data
	if length <= 0 {
		return "", nil
	}

	stringBytes := make([]byte, length)
	page.Read(offset+4, stringBytes)

	return string(stringBytes), nil
}

// SetString sets a string value in the specified field and slot.
func (rp *RecordPage) SetString(slot int, fieldName string, value string) error {
	if !rp.layout.Schema().HasField(fieldName) {
		return ErrInvalidField
	}

	fieldType, _ := rp.layout.Schema().Type(fieldName)
	if fieldType != VarcharType {
		return fmt.Errorf("field %s is not a string", fieldName)
	}

	fieldLength, _ := rp.layout.Schema().Length(fieldName)
	if len(value) > fieldLength {
		return fmt.Errorf("string too long for field %s (max %d chars)", fieldName, fieldLength)
	}

	offset := rp.layout.FieldOffset(slot, fieldName)
	page := rp.buffer.Page()

	// Write 4 bytes for string length
	lengthBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lengthBytes, uint32(len(value)))

	_, err := page.Write(offset, lengthBytes)
	if err != nil {
		return err
	}

	// Write the actual string data
	if len(value) > 0 {
		_, err = page.Write(offset+4, []byte(value))
		if err != nil {
			return err
		}
	}

	rp.buffer.SetDirty(-1) // Mark buffer as dirty
	return nil
}

// IsValidSlot checks if a slot number is valid for this page.
func (rp *RecordPage) IsValidSlot(slot int) bool {
	return rp.layout.IsValidSlot(slot, rp.buffer.Page().Size())
}

// IsInUse checks if a slot is currently in use (contains a valid record).
func (rp *RecordPage) IsInUse(slot int) bool {
	if !rp.IsValidSlot(slot) {
		return false
	}

	offset := rp.layout.SlotOffset(slot)
	page := rp.buffer.Page()

	// Read the 4-byte "in use" flag
	flagBytes := make([]byte, 4)
	page.Read(offset, flagBytes)
	flag := binary.LittleEndian.Uint32(flagBytes)

	return flag != 0
}

// SetInUse sets the "in use" flag for a slot.
func (rp *RecordPage) SetInUse(slot int, inUse bool) error {
	if !rp.IsValidSlot(slot) {
		return fmt.Errorf("invalid slot: %d", slot)
	}

	offset := rp.layout.SlotOffset(slot)
	page := rp.buffer.Page()

	// Write the 4-byte "in use" flag
	flagBytes := make([]byte, 4)
	if inUse {
		binary.LittleEndian.PutUint32(flagBytes, 1)
	} else {
		binary.LittleEndian.PutUint32(flagBytes, 0)
	}

	_, err := page.Write(offset, flagBytes)
	if err != nil {
		return err
	}

	rp.buffer.SetDirty(-1) // Mark buffer as dirty
	return nil
}

// FindFirstAvailableSlot finds the first unused slot in the page.
func (rp *RecordPage) FindFirstAvailableSlot() int {
	maxSlots := rp.layout.SlotsPerBlock(rp.buffer.Page().Size())
	for slot := 0; slot < maxSlots; slot++ {
		if !rp.IsInUse(slot) {
			return slot
		}
	}
	return -1 // No available slots
}

// Format initializes all slots in the page as unused.
func (rp *RecordPage) Format() error {
	maxSlots := rp.layout.SlotsPerBlock(rp.buffer.Page().Size())
	for slot := 0; slot < maxSlots; slot++ {
		err := rp.SetInUse(slot, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// Block returns the block ID of this page.
func (rp *RecordPage) Block() *file.BlockID {
	return rp.block
}
