# SimpleDB Implementation Plan

## Architecture Overview
The current foundation includes:
- **File Manager**: Low-level disk block I/O operations
- **Page**: In-memory representation of disk blocks  
- **Log Manager**: Write-ahead logging with LSN tracking
- **Record**: Log record structure with length-prefixed data

## Implementation Roadmap

### 1. Buffer Manager 🎯 **Next Priority**

#### Components:
```
buffer/
├── buffer.go          # Individual buffer wrapper around Page
├── buffer_pool.go     # Pool of buffers with replacement strategy
├── buffer_mgr.go      # Main buffer manager interface
└── buffer_test.go     # Comprehensive tests
```

#### Key Features:
- **Buffer Pool**: Fixed-size pool of page buffers
- **Replacement Strategy**: LRU (Least Recently Used) algorithm
- **Pin/Unpin Mechanism**: Reference counting for buffer safety
- **Dirty Bit Tracking**: Track modified buffers for write-back
- **Integration with Log Manager**: Ensure WAL (Write-Ahead Logging)

#### Dependencies:
- Uses existing `file.FileMgr` and `file.Page`
- Integrates with `log.LogMgr` for flush coordination

---

### 2. Concurrency Manager

#### Components:
```
concurrency/
├── lock_mgr.go        # Lock manager with lock table
├── lock.go            # Lock types (Shared/Exclusive)
├── tx_table.go        # Active transaction tracking
└── concurrency_test.go
```

#### Key Features:
- **Lock Types**: Shared (S) and Exclusive (X) locks
- **Lock Table**: Block-level locking with wait queues
- **Deadlock Detection**: Simple timeout or wait-for graph
- **Transaction Integration**: Lock acquisition/release tied to TX lifecycle

#### Dependencies:
- Works with Buffer Manager for block access
- Integrates with Recovery Manager for transaction control

---

### 3. Recovery Manager

#### Components:
```
recovery/
├── recovery_mgr.go    # Main recovery coordinator
├── log_records.go     # Different log record types
├── checkpoint.go      # Checkpointing mechanism
└── recovery_test.go
```

#### Key Features:
- **Log Record Types**: START, COMMIT, ABORT, UPDATE, CHECKPOINT
- **UNDO/REDO**: Recovery using log records
- **Checkpointing**: Periodic consistency points
- **Transaction States**: Active, Committed, Aborted tracking

#### Dependencies:
- Heavily uses Log Manager for log record writing/reading
- Coordinates with Buffer Manager for dirty page handling
- Works with Concurrency Manager for transaction state

---

### 4. Record Management

#### Components:
```
record/
├── schema.go          # Table schema definition
├── record_mgr.go      # Record operations (insert/update/delete)
├── record_page.go     # Page-level record management
├── slot.go            # Slot directory for record location
└── record_test.go
```

#### Key Features:
- **Schema Definition**: Column types, names, lengths
- **Variable-Length Records**: Support for VARCHAR fields
- **Slot Directory**: Track record locations within pages
- **Record Operations**: CRUD operations with proper logging

#### Dependencies:
- Uses Buffer Manager for page access
- Integrates with Log Manager for operation logging
- Coordinates with Recovery Manager for transaction safety

---

### 5. Metadata Management

#### Components:
```
metadata/
├── catalog.go         # System catalog management
├── table_mgr.go       # Table metadata operations
├── view_mgr.go        # View definition management
├── stat_mgr.go        # Statistics management
└── metadata_test.go
```

#### Key Features:
- **System Tables**: TABLES, COLUMNS, VIEWS, INDEXES
- **Schema Evolution**: ALTER TABLE operations
- **Statistics**: Cardinality, selectivity for query optimization
- **Metadata Caching**: In-memory metadata cache

#### Dependencies:
- Built on top of Record Management
- Uses all lower-level managers for persistence

---

### 6. Query Processor

#### Components:
```
query/
├── parser/
│   ├── lexer.go       # SQL tokenization
│   ├── parser.go      # SQL parsing
│   └── ast.go         # Abstract syntax tree
├── planner/
│   ├── planner.go     # Query planning
│   ├── optimizer.go   # Cost-based optimization
│   └── plan.go        # Execution plans
├── executor/
│   ├── scan.go        # Scan interface
│   ├── select_scan.go # Selection operation
│   ├── project_scan.go# Projection operation
│   └── join_scan.go   # Join operations
└── query_test.go
```

#### Key Features:
- **SQL Parser**: Support for SELECT, INSERT, UPDATE, DELETE
- **Query Planning**: Generate execution plans
- **Cost Estimation**: Simple cost model for plan selection
- **Execution Engine**: Iterator-based execution model

#### Dependencies:
- Uses Metadata Manager for schema information
- Built on Record Management for data access
- Integrates with all lower-level components

---

### 7. Indexes

#### Components:
```
index/
├── btree/
│   ├── btree.go       # B+ tree implementation
│   ├── btree_page.go  # B+ tree node management
│   └── btree_test.go
├── hash/
│   ├── hash_index.go  # Hash-based indexing
│   └── hash_test.go
├── index_mgr.go       # Index management interface
└── index_test.go
```

#### Key Features:
- **B+ Tree**: Primary indexing structure
- **Hash Index**: Fast equality lookups
- **Index Maintenance**: Automatic index updates
- **Index Selection**: Query optimizer integration

#### Dependencies:
- Uses Record Management for index storage
- Integrates with Query Processor for optimization
- Built on Buffer and File managers

---

### 8. Client Communication

#### Components:
```
client/
├── server.go          # TCP server implementation
├── protocol.go        # Communication protocol
├── session.go         # Client session management
├── jdbc/
│   └── driver.go      # JDBC-style driver interface
└── client_test.go
```

#### Key Features:
- **Network Protocol**: Simple TCP-based protocol
- **Session Management**: Multi-client support
- **JDBC Interface**: Standard database connectivity
- **Transaction Support**: Client-level transaction control

#### Dependencies:
- Top-level component using Query Processor
- Manages transactions across all components

---

## Implementation Timeline

### Phase 1: Core Storage (Weeks 1-2)
1. **Buffer Manager** - Critical for all subsequent components
   - Implement buffer pool with LRU replacement
   - Add comprehensive tests
   - Integration testing with existing File/Log managers

### Phase 2: Transaction Infrastructure (Weeks 3-4)
2. **Concurrency Manager** - Needed before recovery
3. **Recovery Manager** - Transaction safety

### Phase 3: Data Management (Weeks 5-6)
4. **Record Management** - Core data operations
5. **Metadata Management** - Schema management

### Phase 4: Query Processing (Weeks 7-9)
6. **Query Processor** - SQL support
7. **Indexes** - Performance optimization

### Phase 5: Client Interface (Week 10)
8. **Client Communication** - External interface

---

## Key Design Principles

1. **Layered Architecture**: Each component builds on lower layers
2. **Interface-Driven**: Define clear interfaces for testability
3. **Transaction Safety**: All operations must be recoverable
4. **Comprehensive Testing**: Unit and integration tests for each component
5. **Documentation**: Follow the YouTube video format with clear explanations

---

## Dependency Graph

```
Client Communication
       ↓
Query Processor ← Indexes
       ↓              ↓
Metadata Management   ↓
       ↓              ↓
Record Management ←───┘
       ↓
Recovery Manager
       ↓
Concurrency Manager
       ↓
Buffer Manager
       ↓
File Manager + Log Manager (✅ Complete)
```

---

## Testing Strategy

- **Unit Tests**: Each component has comprehensive unit tests
- **Integration Tests**: Cross-component integration testing
- **Performance Tests**: Buffer pool efficiency, query performance
- **Concurrency Tests**: Multi-threaded access patterns
- **Recovery Tests**: Crash recovery scenarios

---

## Documentation Plan

Each component implementation will include:
- Design document explaining the approach
- API documentation with examples
- YouTube video walkthrough
- Performance characteristics
- Known limitations and future improvements