package record

// Layout describes the structure of a record in terms of its fields and their sizes.
type Layout struct {
	schema   *Schema
	offsets  map[string]int // Field name to byte offset mapping
	slotSize int            // Total size of one record slot
}

// NewLayout creates a new layout from a schema.
func NewLayout(schema *Schema) *Layout {
	layout := &Layout{
		schema:  schema,
		offsets: make(map[string]int),
	}

	offset := 4 // Start after the 4-byte "in use" flag

	for _, field := range schema.Fields() {
		layout.offsets[field.Name] = offset

		switch field.Type {
		case IntegerType:
			offset += 4 // Integers are 4 bytes
		case VarcharType:
			offset += 4 + field.Length // 4 bytes for length + actual string data
		}
	}

	layout.slotSize = offset
	return layout
}

// NewLayoutFromOffsets creates a layout with explicit offsets (for existing tables).
func NewLayoutFromOffsets(schema *Schema, offsets map[string]int, slotSize int) *Layout {
	return &Layout{
		schema:   schema,
		offsets:  offsets,
		slotSize: slotSize,
	}
}

// Schema returns the schema for this layout.
func (l *Layout) Schema() *Schema {
	return l.schema
}

// Offset returns the byte offset of the specified field.
func (l *Layout) Offset(fieldName string) int {
	return l.offsets[fieldName]
}

// SlotSize returns the size in bytes of each record slot.
func (l *Layout) SlotSize() int {
	return l.slotSize
}

// SlotsPerBlock calculates how many record slots fit in a block.
func (l *Layout) SlotsPerBlock(blockSize int) int {
	return blockSize / l.slotSize
}

// IsValidSlot checks if a slot number is valid for the given block size.
func (l *Layout) IsValidSlot(slot int, blockSize int) bool {
	return slot >= 0 && slot < l.SlotsPerBlock(blockSize)
}

// SlotOffset calculates the byte offset of a specific slot in a block.
func (l *Layout) SlotOffset(slot int) int {
	return slot * l.slotSize
}

// FieldOffset calculates the byte offset of a field in a specific slot.
func (l *Layout) FieldOffset(slot int, fieldName string) int {
	return l.SlotOffset(slot) + l.Offset(fieldName)
}
