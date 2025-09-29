package record

import (
	"os"
	"testing"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/concurrency"
	"github.com/inelpandzic/simpledb/file"
	"github.com/inelpandzic/simpledb/log"
	"github.com/inelpandzic/simpledb/recovery"
)

func setupRecordTest(t *testing.T) (*Schema, *Layout, *concurrency.ConcurrencyMgr, *buffer.BufferMgr, *recovery.RecoveryMgr, *file.FileMgr, func()) {
	testDir := "testdata/record"
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)

	fm := file.NewFileMgr(testDir, 1024)
	lm := log.NewLogMgr(fm, "testlog")
	bm := buffer.NewBufferMgr(fm, lm, 8)
	cm := concurrency.NewConcurrencyMgr(bm)
	rm := recovery.NewRecoveryMgr(bm, lm, fm)

	// Create a test schema
	schema := NewSchema()
	schema.AddIntField("id")
	schema.AddStringField("name", 20)
	schema.AddIntField("age")

	layout := NewLayout(schema)

	cleanup := func() {
		os.RemoveAll(testDir)
	}

	return schema, layout, cm, bm, rm, fm, cleanup
}

func TestSchema(t *testing.T) {
	schema := NewSchema()

	// Test adding fields
	schema.AddIntField("id")
	schema.AddStringField("name", 50)
	schema.AddIntField("age")

	// Test field existence
	if !schema.HasField("id") {
		t.Error("Schema should have 'id' field")
	}
	if !schema.HasField("name") {
		t.Error("Schema should have 'name' field")
	}
	if !schema.HasField("age") {
		t.Error("Schema should have 'age' field")
	}
	if schema.HasField("nonexistent") {
		t.Error("Schema should not have 'nonexistent' field")
	}

	// Test field types
	idType, err := schema.Type("id")
	if err != nil {
		t.Errorf("Failed to get type for 'id': %v", err)
	}
	if idType != IntegerType {
		t.Error("'id' field should be IntegerType")
	}

	nameType, err := schema.Type("name")
	if err != nil {
		t.Errorf("Failed to get type for 'name': %v", err)
	}
	if nameType != VarcharType {
		t.Error("'name' field should be VarcharType")
	}

	// Test field length
	nameLength, err := schema.Length("name")
	if err != nil {
		t.Errorf("Failed to get length for 'name': %v", err)
	}
	if nameLength != 50 {
		t.Errorf("'name' field length should be 50, got %d", nameLength)
	}
}

func TestLayout(t *testing.T) {
	schema := NewSchema()
	schema.AddIntField("id")
	schema.AddStringField("name", 20)
	schema.AddIntField("age")

	layout := NewLayout(schema)

	// Test slot size calculation
	// Expected: 4 (in-use flag) + 4 (id) + 4 + 20 (name) + 4 (age) = 36 bytes
	expectedSize := 4 + 4 + 4 + 20 + 4
	if layout.SlotSize() != expectedSize {
		t.Errorf("Expected slot size %d, got %d", expectedSize, layout.SlotSize())
	}

	// Test slots per block
	blockSize := 1024
	expectedSlots := blockSize / expectedSize
	if layout.SlotsPerBlock(blockSize) != expectedSlots {
		t.Errorf("Expected %d slots per block, got %d", expectedSlots, layout.SlotsPerBlock(blockSize))
	}

	// Test field offsets
	if layout.Offset("id") != 4 {
		t.Errorf("Expected 'id' offset 4, got %d", layout.Offset("id"))
	}
	if layout.Offset("name") != 8 {
		t.Errorf("Expected 'name' offset 8, got %d", layout.Offset("name"))
	}
	if layout.Offset("age") != 32 {
		t.Errorf("Expected 'age' offset 32, got %d", layout.Offset("age"))
	}
}

func TestRID(t *testing.T) {
	block := file.NewBlockID("test.tbl", 5)
	rid1 := NewRID(block, 10)
	rid2 := NewRID(block, 10)
	rid3 := NewRID(block, 15)

	// Test accessors
	if rid1.Block().Filename != "test.tbl" {
		t.Error("RID block filename should be 'test.tbl'")
	}
	if rid1.Block().Number != 5 {
		t.Error("RID block number should be 5")
	}
	if rid1.Slot() != 10 {
		t.Error("RID slot should be 10")
	}

	// Test equality
	if !rid1.Equals(rid2) {
		t.Error("rid1 should equal rid2")
	}
	if rid1.Equals(rid3) {
		t.Error("rid1 should not equal rid3")
	}
	if rid1.Equals(nil) {
		t.Error("rid1 should not equal nil")
	}

	// Test string representation
	expected := "RID[test.tbl:5, slot=10]"
	if rid1.String() != expected {
		t.Errorf("Expected RID string '%s', got '%s'", expected, rid1.String())
	}
}

func TestRecordPage(t *testing.T) {
	_, layout, _, _, _, fm, cleanup := setupRecordTest(t)
	defer cleanup()

	// Create a block and empty page
	block := file.NewBlockID("testfile", 0)
	emptyPage := file.NewPage(fm.BlockSize)

	// Write empty page to create the block
	_, err := fm.Write(block, emptyPage)
	if err != nil {
		t.Fatalf("Failed to write block: %v", err)
	}

	// Verify layout slot size calculation
	if layout.SlotSize() <= 0 {
		t.Errorf("Expected positive slot size, got %d", layout.SlotSize())
	}

	// Verify slots per block calculation
	slotsPerBlock := layout.SlotsPerBlock(fm.BlockSize)
	if slotsPerBlock <= 0 {
		t.Errorf("Expected positive slots per block, got %d", slotsPerBlock)
	}

	t.Logf("Layout: SlotSize=%d, SlotsPerBlock=%d", layout.SlotSize(), slotsPerBlock)
}

func TestTableScan(t *testing.T) {
	_, layout, cm, bm, rm, _, cleanup := setupRecordTest(t)
	defer cleanup()

	// Create a transaction
	tx := cm.BeginTransaction()
	defer cm.CommitTransaction(tx)

	// Create table scan
	ts := NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	// Insert some test records
	records := []struct {
		id   int
		name string
		age  int
	}{
		{1, "Alice", 20},
		{2, "Bob", 22},
		{3, "Charlie", 19},
	}

	for _, record := range records {
		err := ts.Insert()
		if err != nil {
			t.Fatalf("Failed to insert record: %v", err)
		}

		err = ts.SetInt("id", record.id)
		if err != nil {
			t.Fatalf("Failed to set id: %v", err)
		}

		err = ts.SetString("name", record.name)
		if err != nil {
			t.Fatalf("Failed to set name: %v", err)
		}

		err = ts.SetInt("age", record.age)
		if err != nil {
			t.Fatalf("Failed to set age: %v", err)
		}
	}

	// Close and reopen the scan to test persistence
	ts.Close()
	ts = NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	// Read back the records
	recordCount := 0
	for ts.HasData() {
		id, err := ts.GetInt("id")
		if err != nil {
			t.Fatalf("Failed to get id: %v", err)
		}

		name, err := ts.GetString("name")
		if err != nil {
			t.Fatalf("Failed to get name: %v", err)
		}

		age, err := ts.GetInt("age")
		if err != nil {
			t.Fatalf("Failed to get age: %v", err)
		}

		t.Logf("Record %d: id=%d, name=%s, age=%d", recordCount, id, name, age)

		// Verify the data matches what we inserted
		if recordCount < len(records) {
			expected := records[recordCount]
			if id != expected.id || name != expected.name || age != expected.age {
				t.Errorf("Record %d mismatch: expected (id=%d, name=%s, age=%d), got (id=%d, name=%s, age=%d)",
					recordCount, expected.id, expected.name, expected.age, id, name, age)
			}
		}

		recordCount++
		ts.Next()
	}

	if recordCount != len(records) {
		t.Errorf("Expected %d records, got %d", len(records), recordCount)
	}
}
