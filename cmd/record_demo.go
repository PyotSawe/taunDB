package main

import (
	"fmt"
	"log"
	"os"

	"github.com/inelpandzic/simpledb/buffer"
	"github.com/inelpandzic/simpledb/concurrency"
	"github.com/inelpandzic/simpledb/file"
	logger "github.com/inelpandzic/simpledb/log"
	"github.com/inelpandzic/simpledb/record"
	"github.com/inelpandzic/simpledb/recovery"
)

func recordDemo() {
	fmt.Println("=== SimpleDB Record Management Demo ===")
	fmt.Println()

	// Setup - create temporary directory
	dataDir := "/tmp/simpledb_record_demo"
	os.RemoveAll(dataDir)
	os.MkdirAll(dataDir, 0755)
	defer os.RemoveAll(dataDir)

	// Initialize database components
	fmt.Println("1. Initializing database components...")
	fm := file.NewFileMgr(dataDir, 1024)
	lm := logger.NewLogMgr(fm, "logfile")
	bm := buffer.NewBufferMgr(fm, lm, 10)
	cm := concurrency.NewConcurrencyMgr(bm)
	rm := recovery.NewRecoveryMgr(bm, lm, fm)

	// Create a schema for a student table
	fmt.Println("2. Creating schema for student table...")
	schema := record.NewSchema()
	schema.AddIntField("id")
	schema.AddStringField("name", 20)
	schema.AddStringField("major", 15)
	schema.AddIntField("grad_year")

	// Create layout from schema
	layout := record.NewLayout(schema)
	fmt.Printf("   Schema created with %d fields\n", len(schema.Fields()))
	fmt.Printf("   Record slot size: %d bytes\n", layout.SlotSize())
	fmt.Printf("   Records per block: %d\n", layout.SlotsPerBlock(fm.BlockSize))
	fmt.Println()

	// Start a transaction
	fmt.Println("3. Starting transaction...")
	tx := cm.BeginTransaction()
	defer cm.CommitTransaction(tx)

	// Create a table scan
	fmt.Println("4. Creating table scan for 'students' table...")
	ts := record.NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	// Insert student records
	fmt.Println("5. Inserting student records...")
	students := []struct {
		id       int
		name     string
		major    string
		gradYear int
	}{
		{1, "Alice Johnson", "Computer Science", 2024},
		{2, "Bob Smith", "Mathematics", 2025},
		{3, "Carol Davis", "Physics", 2024},
		{4, "David Wilson", "Computer Science", 2026},
		{5, "Eve Brown", "Chemistry", 2025},
	}

	for _, student := range students {
		if err := ts.Insert(); err != nil {
			log.Fatalf("Failed to insert record: %v", err)
		}

		if err := ts.SetInt("id", student.id); err != nil {
			log.Fatalf("Failed to set id: %v", err)
		}
		if err := ts.SetString("name", student.name); err != nil {
			log.Fatalf("Failed to set name: %v", err)
		}
		if err := ts.SetString("major", student.major); err != nil {
			log.Fatalf("Failed to set major: %v", err)
		}
		if err := ts.SetInt("grad_year", student.gradYear); err != nil {
			log.Fatalf("Failed to set grad_year: %v", err)
		}

		fmt.Printf("   Inserted: %s (ID: %d, Major: %s, Grad: %d)\n",
			student.name, student.id, student.major, student.gradYear)
	}
	fmt.Printf("   Total records inserted: %d\n", len(students))
	fmt.Println()

	// Close and reopen to test persistence
	ts.Close()
	fmt.Println("6. Reopening table scan to test persistence...")
	ts = record.NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	// Read and display all records
	fmt.Println("7. Reading all student records:")
	fmt.Println("   ID | Name               | Major           | Grad Year")
	fmt.Println("   ---|--------------------|-----------------|---------")

	recordCount := 0
	for ts.HasData() {
		id, err := ts.GetInt("id")
		if err != nil {
			log.Fatalf("Failed to get ID: %v", err)
		}

		name, err := ts.GetString("name")
		if err != nil {
			log.Fatalf("Failed to get name: %v", err)
		}

		major, err := ts.GetString("major")
		if err != nil {
			log.Fatalf("Failed to get major: %v", err)
		}

		gradYear, err := ts.GetInt("grad_year")
		if err != nil {
			log.Fatalf("Failed to get grad_year: %v", err)
		}

		fmt.Printf("   %2d | %-18s | %-15s | %d\n", id, name, major, gradYear)
		recordCount++

		if !ts.Next() {
			break
		}
	}

	fmt.Printf("\n   Total records read: %d\n", recordCount)
	fmt.Println()

	// Demonstrate updates
	fmt.Println("8. Updating Alice Johnson's graduation year...")
	ts.Close()
	ts = record.NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	// Find Alice and update her graduation year
	for ts.HasData() {
		name, err := ts.GetString("name")
		if err != nil {
			log.Fatalf("Failed to get name: %v", err)
		}

		if name == "Alice Johnson" {
			if err := ts.SetInt("grad_year", 2023); err != nil {
				log.Fatalf("Failed to update grad_year: %v", err)
			}
			fmt.Println("   Updated Alice Johnson's graduation year to 2023")
			break
		}

		if !ts.Next() {
			break
		}
	}

	// Verify the update
	ts.Close()
	ts = record.NewTableScan(tx, "students", layout, bm, rm)
	defer ts.Close()

	fmt.Println("9. Verifying update - Alice's record:")
	for ts.HasData() {
		name, err := ts.GetString("name")
		if err != nil {
			log.Fatalf("Failed to get name: %v", err)
		}

		if name == "Alice Johnson" {
			id, _ := ts.GetInt("id")
			major, _ := ts.GetString("major")
			gradYear, _ := ts.GetInt("grad_year")
			fmt.Printf("   %d | %s | %s | %d\n", id, name, major, gradYear)
			break
		}

		if !ts.Next() {
			break
		}
	}

	fmt.Println()
	fmt.Println("=== Record Management Demo Complete ===")
	fmt.Println("Successfully demonstrated:")
	fmt.Println("  ✓ Schema creation and layout calculation")
	fmt.Println("  ✓ Table creation and record insertion")
	fmt.Println("  ✓ Data persistence across table scan sessions")
	fmt.Println("  ✓ Record retrieval and iteration")
	fmt.Println("  ✓ Record updates with transaction safety")
	fmt.Println("  ✓ Integration with Buffer and Recovery managers")
}

// Uncomment the main function below to run the demo
// func main() {
// 	recordDemo()
// }
