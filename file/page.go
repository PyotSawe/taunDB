package file

import (
	"encoding/binary"
	"errors"
)

type Page struct {
	bytes []byte
}

// NewPage creates a new page with the specified size.
func NewPage(size int) *Page {
	return &Page{
		bytes: make([]byte, size),
	}
}

// Write copies data from the data slice to the page at the specified offset.
func (p *Page) Write(offset int, data []byte) (int, error) {
	if offset+len(data) > p.Size() {
		return 0, errors.New("data exceeds page bounds")
	}

	n := copy(p.bytes[offset:], data)
	return n, nil
}

// WriteInt writes an integer value to the page at the specified offset.
func (p *Page) WriteInt(offset int, value int) error {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, uint32(value))

	_, err := p.Write(offset, b)
	return err
}

// Read copies data from the page at the specified offset and writes it to the data slice.
func (p *Page) Read(offset int, data []byte) int {
	return copy(data, p.bytes[offset:])
}

// Bytes returns the byte of the page.
func (p *Page) Bytes() []byte {
	return p.bytes
}

// Contents returns the contents of the page (alias for Bytes)
func (p *Page) Contents() []byte {
	return p.bytes
}

func (p *Page) Size() int {
	return len(p.bytes)
}
