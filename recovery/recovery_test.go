package recovery

import (
	"os"
	"testing"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
)

func setupRecoveryTest(t *testing.T) (*RecoveryMgr, *buffer.BufferMgr, *log.LogMgr, *file.FileMgr, func()) {
	testDir := "testdata/recovery"
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)

	fm := file.NewFileMgr(testDir, 1024)
	lm := log.NewLogMgr(fm, "testlog")

	bm := buffer.NewBufferMgr(fm, lm, 8)

	// Create recovery manager without auto-recovery for testing
	rm := &RecoveryMgr{
		bm:                bm,
		lm:                lm,
		fm:                fm,
		lastCheckpointLSN: -1,
	}

	cleanup := func() {
		os.RemoveAll(testDir)
	}

	return rm, bm, lm, fm, cleanup
}

func TestRecoveryMgr_WriteLogRecords(t *testing.T) {
	rm, _, _, _, cleanup := setupRecoveryTest(t)
	defer cleanup()

	// Test writing different types of log records
	lsn1, err := rm.WriteStartRecord(1)
	if err != nil {
		t.Fatalf("Failed to write start record: %v", err)
	}
	if lsn1 < 0 {
		t.Error("Invalid LSN returned for start record")
	}

	block := file.NewBlockID("testfile", 0)
	oldValue := []byte("old data")
	newValue := []byte("new data")

	lsn2, err := rm.WriteUpdateRecord(1, block, 0, oldValue, newValue)
	if err != nil {
		t.Fatalf("Failed to write update record: %v", err)
	}
	if lsn2 <= lsn1 {
		t.Error("UPDATE record LSN should be greater than START record")
	}

	lsn3, err := rm.WriteCommitRecord(1)
	if err != nil {
		t.Fatalf("Failed to write commit record: %v", err)
	}
	if lsn3 <= lsn2 {
		t.Error("COMMIT record LSN should be greater than UPDATE record")
	}
}

func TestRecoveryMgr_Checkpoint(t *testing.T) {
	rm, _, _, _, cleanup := setupRecoveryTest(t)
	defer cleanup()

	// Start some transactions
	rm.WriteStartRecord(1)
	rm.WriteStartRecord(2)
	rm.WriteStartRecord(3)

	// Create checkpoint with active transactions
	activeTxs := []int{1, 2, 3}
	err := rm.Checkpoint(activeTxs)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	if rm.lastCheckpointLSN < 0 {
		t.Error("Checkpoint LSN should be set after checkpoint")
	}
}

func TestRecoveryMgr_Recovery(t *testing.T) {
	rm, bm, _, _, cleanup := setupRecoveryTest(t)
	defer cleanup()

	// Simulate some transactions
	block := file.NewBlockID("testfile", 0)

	// Create the test file first by creating an empty page
	emptyPage := file.NewPage(1024)
	originalData := []byte("original data")
	copy(emptyPage.Contents()[0:], originalData)

	// Write the page to create the file
	_, err := rm.fm.Write(block, emptyPage)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Transaction 1: Start and commit
	rm.WriteStartRecord(1)
	oldValue := make([]byte, len(originalData))
	copy(oldValue, originalData)
	newValue := []byte("updated data1")
	rm.WriteUpdateRecord(1, block, 0, oldValue, newValue)
	rm.WriteCommitRecord(1)

	// Transaction 2: Start but don't commit (will be aborted during recovery)
	rm.WriteStartRecord(2)
	oldValue2 := make([]byte, len(newValue))
	copy(oldValue2, newValue)
	newValue2 := []byte("updated data2")
	rm.WriteUpdateRecord(2, block, 0, oldValue2, newValue2)

	// Perform recovery
	err = rm.Recover()
	if err != nil {
		t.Fatalf("Recovery failed: %v", err)
	}

	// Verify that committed transaction changes are preserved
	// and uncommitted transaction changes are rolled back
	buf, err := bm.Pin(block)
	if err != nil {
		t.Fatalf("Failed to pin buffer after recovery: %v", err)
	}
	defer bm.Unpin(buf)

	// The data should be the result of transaction 1 (committed)
	// Transaction 2 should be undone
	page := buf.Page()
	resultData := string(page.Contents()[0:len(newValue)])
	if resultData != string(newValue) {
		t.Errorf("Expected data after recovery: %s, got: %s", string(newValue), resultData)
	}
}

func TestLogRecord_Types(t *testing.T) {
	// Test StartRecord
	start := NewStartRecord(1, 100)
	if start.Type() != START {
		t.Error("StartRecord should have START type")
	}
	if start.TxNum() != 1 {
		t.Error("StartRecord should have correct transaction number")
	}

	// Test CommitRecord
	commit := NewCommitRecord(1, 200)
	if commit.Type() != COMMIT {
		t.Error("CommitRecord should have COMMIT type")
	}

	// Test AbortRecord
	abort := NewAbortRecord(1, 300)
	if abort.Type() != ABORT {
		t.Error("AbortRecord should have ABORT type")
	}

	// Test UpdateRecord
	block := file.NewBlockID("test", 0)
	oldVal := []byte("old")
	newVal := []byte("new")
	update := NewUpdateRecord(1, 400, block, 0, oldVal, newVal)

	if update.Type() != UPDATE {
		t.Error("UpdateRecord should have UPDATE type")
	}
	if update.Block().Filename != "test" {
		t.Error("UpdateRecord should have correct block")
	}
	if string(update.OldValue()) != "old" {
		t.Error("UpdateRecord should have correct old value")
	}
	if string(update.NewValue()) != "new" {
		t.Error("UpdateRecord should have correct new value")
	}

	// Test CheckpointRecord
	activeTxs := []int{1, 2, 3}
	checkpoint := NewCheckpointRecord(500, activeTxs)
	if checkpoint.Type() != CHECKPOINT {
		t.Error("CheckpointRecord should have CHECKPOINT type")
	}
	if len(checkpoint.ActiveTxs()) != 3 {
		t.Error("CheckpointRecord should have correct active transactions")
	}
}
