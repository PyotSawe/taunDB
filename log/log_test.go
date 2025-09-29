package log

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/inelpandzic/simpledb/file"
)

func TestNewMgr(t *testing.T) {
	dataDir := "testdata"
	logFile := "testlogfile"

	// Create test directory if it doesn't exist
	os.MkdirAll(dataDir, 0755)

	fileMgr := file.NewFileMgr(dataDir, 32)
	t.Cleanup(func() {
		fileMgr.Close()
		os.Remove(filepath.Join(dataDir, logFile))
	})

	logMgr := NewLogMgr(fileMgr, logFile)

	offsetBytes := make([]byte, 4)
	logMgr.logPage.Read(0, offsetBytes)
	offset := endian.Uint32(offsetBytes)
	if offset != 32 {
		t.Errorf("offset = %d, want %d", offset, 32)
	}

	logSize, err := fileMgr.FileSize(logFile)
	if err != nil {
		t.Fatalf("FileSize failed: %v", err)
	}

	if logSize != 1 {
		t.Errorf("logSize = %d, want %d", logSize, 1)
	}

	// Test when log file already exists
	fileMgr.Write(&file.BlockID{
		Filename: logFile,
		Number:   1,
	}, file.NewPage(fileMgr.BlockSize))

	_ = NewLogMgr(fileMgr, logFile)

	logSize, err = fileMgr.FileSize(logFile)
	if err != nil {
		t.Fatalf("FileSize failed: %v", err)
	}

	if logSize != 2 {
		t.Errorf("logSize = %d, want %d", logSize, 2)
	}
}

func TestLog(t *testing.T) {
	dataDir := "testdata"
	logFile := "testlogfile1"

	fileMgr := file.NewFileMgr(dataDir, 32)
	t.Cleanup(func() {
		fileMgr.Close()
		os.Remove(filepath.Join(dataDir, logFile))
	})

	logMgr := NewLogMgr(fileMgr, logFile)

	tests := []struct {
		name            string
		record          *Record
		expectedLogSize int
		expectedOffset  int
		expectedLSN     int
	}{
		{
			name:            "test loging first record",
			record:          NewRecord([]byte("test record")),
			expectedLogSize: 1,
			expectedOffset:  17, // 32 (offset before the write) - 15 (4 bytes for length and 11 bytes for data)
			expectedLSN:     1,
		},
		{
			name:            "test logging second record to be flushed to the same first block",
			record:          NewRecord([]byte("record 2")),
			expectedLogSize: 1,
			expectedOffset:  5, // 17 (offset before the write) - 12 (4 bytes for length and 8 bytes for data)
			expectedLSN:     2,
		},
		{
			name:            "test logging third record to be flushed to the new second block",
			record:          NewRecord([]byte("record 3")),
			expectedLogSize: 2,
			expectedOffset:  20, // 32 (new block - offset before the write) - 12 (4 bytes for length and 8 bytes for data)
			expectedLSN:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lsn, err := logMgr.Log(tt.record)
			if err != nil {
				t.Fatalf("Log failed: %v", err)
			}

			if lsn != tt.expectedLSN {
				t.Errorf("lsn = %d, want %d", lsn, tt.expectedLSN)
			}

			offsetBytes := make([]byte, 4)
			logMgr.logPage.Read(0, offsetBytes)
			offset := binary.NativeEndian.Uint32(offsetBytes)

			if offset != uint32(tt.expectedOffset) {
				t.Errorf("offset = %d, want %d", offset, tt.expectedOffset)
			}

			logSize, err := fileMgr.FileSize(logFile)
			if err != nil {
				t.Fatalf("FileSize failed: %v", err)
			}
			if logSize != tt.expectedLogSize {
				t.Errorf("logSize = %d, want %d", logSize, tt.expectedLogSize)
			}
		})
	}
}

func TestIterator(t *testing.T) {
	dataDir := "testdata"
	logFile := "testlogfile2"

	fileMgr := file.NewFileMgr(dataDir, 32)
	t.Cleanup(func() {
		fileMgr.Close()
		os.Remove(filepath.Join(dataDir, logFile))
	})

	logMgr := NewLogMgr(fileMgr, logFile)

	records := []*Record{
		NewRecord([]byte("record one")),
		NewRecord([]byte("record two")),
		NewRecord([]byte("record three")),
		NewRecord([]byte("record four")),
		NewRecord([]byte("record five")),
		NewRecord([]byte("record six")),
		NewRecord([]byte("record seven")),
		NewRecord([]byte("record eight")),
		NewRecord([]byte("record nine")),
	}

	for _, record := range records {
		_, err := logMgr.Log(record)
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	iter, err := logMgr.Iterator()
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}

	for i := 8; i >= 0; i-- {
		if !iter.HasNext() {
			t.Fatalf("HasNext returned false, want true")
		}

		rec, err := iter.Next()
		if err != nil {
			t.Fatalf("Next failed: %v", err)
		}

		if string(rec.Data) != string(records[i].Data) {
			t.Errorf("record data, got = %s, want %s", rec.Data, records[i].Data)
		}
	}
}
