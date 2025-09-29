package log

import (
	"encoding/binary"
	"math"
)

type Record struct {
	Length int
	Data   []byte
	pos    int // Reading position for NewRecordFromData
}

func NewRecord(data []byte) *Record {
	return &Record{
		Length: len(data),
		Data:   data,
		pos:    0,
	}
}

// NewRecordBuilder creates a new empty record for building
func NewRecordBuilder() *Record {
	return &Record{
		Length: 0,
		Data:   make([]byte, 0, 1024),
		pos:    0,
	}
}

// NewRecordFromData creates a record from existing data for reading
func NewRecordFromData(data []byte) *Record {
	return &Record{
		Length: len(data),
		Data:   data,
		pos:    0,
	}
}

// WriteInt writes an integer to the record
func (r *Record) WriteInt(val int) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(val))
	r.Data = append(r.Data, bytes...)
	r.Length = len(r.Data)
}

// ReadInt reads an integer from the record
func (r *Record) ReadInt() int {
	if r.pos+4 > len(r.Data) {
		return 0
	}
	val := binary.LittleEndian.Uint32(r.Data[r.pos : r.pos+4])
	r.pos += 4
	return int(val)
}

// WriteFloat64 writes a float64 to the record
func (r *Record) WriteFloat64(val float64) {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, math.Float64bits(val))
	r.Data = append(r.Data, bytes...)
	r.Length = len(r.Data)
}

// ReadFloat64 reads a float64 from the record
func (r *Record) ReadFloat64() float64 {
	if r.pos+8 > len(r.Data) {
		return 0.0
	}
	bits := binary.LittleEndian.Uint64(r.Data[r.pos : r.pos+8])
	r.pos += 8
	return math.Float64frombits(bits)
}

// WriteString writes a string to the record
func (r *Record) WriteString(val string) {
	// Write length first
	r.WriteInt(len(val))
	// Write string bytes
	r.Data = append(r.Data, []byte(val)...)
	r.Length = len(r.Data)
}

// ReadString reads a string from the record
func (r *Record) ReadString() string {
	length := r.ReadInt()
	if length <= 0 || r.pos+length > len(r.Data) {
		return ""
	}
	str := string(r.Data[r.pos : r.pos+length])
	r.pos += length
	return str
}

// WriteBytes writes a byte array to the record
func (r *Record) WriteBytes(val []byte) {
	r.Data = append(r.Data, val...)
	r.Length = len(r.Data)
}

// ReadBytes reads a byte array of specified length from the record
func (r *Record) ReadBytes(length int) []byte {
	if length <= 0 || r.pos+length > len(r.Data) {
		return nil
	}
	bytes := make([]byte, length)
	copy(bytes, r.Data[r.pos:r.pos+length])
	r.pos += length
	return bytes
}

// GetData returns the complete record data
func (r *Record) GetData() []byte {
	return r.Data
}

// Reset resets the read position to the beginning
func (r *Record) Reset() {
	r.pos = 0
}

// Size returns the size of the record in bytes
func (r *Record) Size() int {
	return len(r.Data)
}

// bytes returns whole record bytes, length 4-byte metadata field plus data.
func (r *Record) bytes() []byte {
	lengthBytes := make([]byte, intBytesSize)
	endian.PutUint32(lengthBytes, uint32(r.Length))

	return append(lengthBytes, r.Data...)
}

// totalLength returns the total length of the record, including the length 4-byte metadata field.
func (r *Record) totalLength() int {
	return intBytesSize + r.Length
}
