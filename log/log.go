package log

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/inelpandzic/simpledb/file"
)

var endian = binary.NativeEndian

// intBytesSize is the size of the integer in bytes.
const intBytesSize = 4

type LogMgr struct {
	logFile          string
	fm               *file.FileMgr
	logPage          *file.Page
	currentBlock     *file.BlockID
	latestLSN        int
	latestDurableLSN int

	mu sync.Mutex
}

func NewLogMgr(fm *file.FileMgr, logFile string) *LogMgr {
	currentBlock := &file.BlockID{
		Filename: logFile,
		Number:   0,
	}

	logPage := file.NewPage(fm.BlockSize)

	logSize, err := fm.FileSize(logFile)
	if err != nil {
		panic(err)
	}

	if logSize == 0 {
		// initial offset is the block size
		err := logPage.WriteInt(0, fm.BlockSize)
		if err != nil {
			panic(err)
		}

		_, err = fm.Write(currentBlock, logPage)
		if err != nil {
			panic(err)
		}
	} else {
		currentBlock.Number = logSize - 1

		_, err = fm.Read(currentBlock, logPage)
		if err != nil {
			panic(err)
		}
	}

	return &LogMgr{
		fm:           fm,
		logFile:      logFile,
		logPage:      logPage,
		currentBlock: currentBlock,
	}
}

// Log logs the record to the log page.
func (lm *LogMgr) Log(record *Record) (int, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	offsetBytes := make([]byte, intBytesSize)
	lm.logPage.Read(0, offsetBytes)

	offset := endian.Uint32(offsetBytes) // offset where the last record starts

	// Buffer capacity, the space left in the log page,
	// is the offset minus the bytes that hold the offset itself.
	buffCap := int(offset) - intBytesSize
	if record.totalLength() > buffCap {
		err := lm.Flush()
		if err != nil {
			return 0, err
		}

		// We can reuse the existing log page and overwrite it with the new data,
		// but a fresh page is created here for simplicity and ease of testing and inspection.
		// The old page will be garbage collected.
		lm.logPage = file.NewPage(lm.fm.BlockSize)

		// create a new blockID by incrementing the block number
		lm.currentBlock.Number++
		_, err = lm.fm.Write(lm.currentBlock, lm.logPage)
		if err != nil {
			return 0, fmt.Errorf("failed to write log page: %w", err)
		}

		offset = uint32(lm.fm.BlockSize)
		err = lm.logPage.WriteInt(0, int(offset))
		if err != nil {
			return 0, fmt.Errorf("failed to write new offset: %w", err)
		}
	}

	recPos := int(offset) - record.totalLength()

	_, err := lm.logPage.Write(recPos, record.bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to write record: %w", err)
	}

	newOffset := recPos
	err = lm.logPage.WriteInt(0, newOffset)
	if err != nil {
		return 0, fmt.Errorf("failed to write new offset: %w", err)
	}

	lm.latestLSN++

	return lm.latestLSN, nil
}

// Append appends raw data to the log (alias for Log with data)
func (lm *LogMgr) Append(data []byte) (int, error) {
	record := NewRecord(data)
	return lm.Log(record)
}

// Iterator returns an iterator that iterates over the log records in reverse order.
func (lm *LogMgr) Iterator() (*iterator, error) {
	err := lm.Flush()
	if err != nil {
		return nil, err
	}

	return newIterator(lm.fm, *lm.currentBlock)
}

// Flush writes the log page to disk.
func (lm *LogMgr) Flush() error {
	_, err := lm.fm.Write(lm.currentBlock, lm.logPage)
	if err != nil {
		return fmt.Errorf("failed to flush log page: %w", err)
	}

	lm.latestDurableLSN = lm.latestLSN

	return nil
}
