package record

import (
	"errors"
)

var (
	ErrInvalidField   = errors.New("invalid field")
	ErrRecordTooLarge = errors.New("record too large for page")
)

// FieldType represents the type of a field.
type FieldType int

const (
	IntegerType FieldType = iota
	VarcharType
)

// FieldInfo describes a field in a table schema.
type FieldInfo struct {
	Name   string
	Type   FieldType
	Length int // For VARCHAR fields
}

// Schema represents a table schema.
type Schema struct {
	fields   []FieldInfo
	fieldMap map[string]int // Maps field name to index
}

// NewSchema creates a new empty schema.
func NewSchema() *Schema {
	return &Schema{
		fields:   make([]FieldInfo, 0),
		fieldMap: make(map[string]int),
	}
}

// AddIntField adds an integer field to the schema.
func (s *Schema) AddIntField(fieldName string) {
	s.addField(fieldName, IntegerType, 0)
}

// AddStringField adds a string field to the schema.
func (s *Schema) AddStringField(fieldName string, length int) {
	s.addField(fieldName, VarcharType, length)
}

// addField adds a field to the schema.
func (s *Schema) addField(name string, fieldType FieldType, length int) {
	field := FieldInfo{
		Name:   name,
		Type:   fieldType,
		Length: length,
	}
	s.fieldMap[name] = len(s.fields)
	s.fields = append(s.fields, field)
}

// Fields returns all fields in the schema.
func (s *Schema) Fields() []FieldInfo {
	return s.fields
}

// HasField returns true if the schema contains the specified field.
func (s *Schema) HasField(fieldName string) bool {
	_, exists := s.fieldMap[fieldName]
	return exists
}

// Type returns the type of the specified field.
func (s *Schema) Type(fieldName string) (FieldType, error) {
	if index, exists := s.fieldMap[fieldName]; exists {
		return s.fields[index].Type, nil
	}
	return IntegerType, ErrInvalidField
}

// Length returns the length of the specified field.
func (s *Schema) Length(fieldName string) (int, error) {
	if index, exists := s.fieldMap[fieldName]; exists {
		return s.fields[index].Length, nil
	}
	return 0, ErrInvalidField
}
