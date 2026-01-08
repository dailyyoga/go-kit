# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a Go toolkit library (`github.com/dailyyoga/nexgo`) providing seven core packages for common infrastructure patterns:

- **logger** - Unified logging interface based on zap with configurable levels, encoding, and output
- **db** - MySQL database client wrapper with connection pool management, structured logging, and slow query detection
- **ch** - ClickHouse client with unified query/write operations and async batch writing
- **kafka** - Kafka consumer/producer wrappers with retry, parallel processing, and error handling
- **cron** - Cron job manager with chain-based task execution and middleware support
- **routine** - Safe goroutine execution with panic recovery to prevent application crashes
- **cache** - Syncable cache with periodic data synchronization, automatic retry logic, and Redis client wrapper

All packages use interface-driven design for testability. The logger package provides a unified `Logger` interface used across all other packages.

## Development Commands

### Running Tests
```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./logger
go test ./db
go test ./ch
go test ./kafka
go test ./cron
go test ./routine
go test ./cache

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
```

### Building
```bash
# Verify all packages compile
go build ./...

# Check dependencies
go mod tidy
go mod verify
```

### Code Quality
```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...

# Run staticcheck (if available)
staticcheck ./...
```

## Architecture

### Package: logger (Logging Interface)

**Core Pattern**: Unified logging interface based on zap with configurable output, encoding, and levels. Supports both dependency injection and global package-level functions.

**Key Components**:
- `Logger` interface - Standard logging interface with `Debug()`, `Info()`, `Warn()`, `Error()`, `Sync()`
- `Config` struct - Configuration for log level, encoding, and output paths
- `New(cfg)` factory - Creates a new logger instance and automatically sets it as the global logger
- Package-level functions - `Debug()`, `Info()`, `Warn()`, `Error()`, `Sync()` for convenient global access
- `SetGlobalLogger(l)` - Manually set the global logger
- `GetGlobalLogger()` - Retrieve the current global logger

**Architecture Details**:
- Built on `uber-go/zap` for high-performance structured logging
- Supports two encoding formats: JSON (default) and Console
- Configurable log levels: debug, info, warn, error, dpanic, panic, fatal
- Configurable output paths for normal logs and error logs
- Automatic default value merging for partial configurations
- Validation ensures only valid log levels and encodings are accepted
- **Global Logger**: `New()` automatically sets a global logger with correct `CallerSkip` for package-level functions
- **Concurrency Safe**: Global logger access is protected by `sync.RWMutex`
- **Lazy Initialization**: If `New()` is never called, package-level functions use a default logger

**Important Files**:
- `logger.go` - Logger interface and factory function
- `global.go` - Global logger instance and package-level functions
- `config.go` - Configuration with validation and defaults
- `errors.go` - Error constructors for invalid configuration

**Configuration**:
```go
cfg := &logger.Config{
    Level:            "info",            // Log level
    Encoding:         "json",            // "json" or "console"
    OutputPaths:      []string{"stdout"}, // Output destinations
    ErrorOutputPaths: []string{"stderr"}, // Error output destinations
}
log, err := logger.New(cfg)
```

**Usage** (Two Ways):
```go
import "github.com/dailyyoga/nexgo/logger"

func main() {
    // Create logger - automatically sets global logger
    log, err := logger.New(&logger.Config{Level: "debug"})
    if err != nil {
        panic(err)
    }
    defer logger.Sync()

    // Way 1: Dependency Injection (recommended for libraries/components)
    svc := NewService(log)
    log.Info("using DI logger")

    // Way 2: Global functions (convenient for application code)
    logger.Info("using global logger")
    logger.Error("error occurred", zap.Error(err))
}
```

**Usage Across Packages**:
All other packages (db, ch, kafka, cron, routine, cache) accept `logger.Logger` interface in their constructors:
- `db.NewMySQL(logger, config)`
- `ch.NewClient(config, logger)`
- `kafka.NewConsumer(logger, config)`
- `kafka.NewProducer(logger, config)`
- `cron.NewCron(logger, middlewares...)`
- `routine.New(logger)`
- `cache.NewSyncableCache(logger, config, syncFunc)`
- `cache.NewRedis(logger, config)`

### Package: db (MySQL Database Client)

**Core Pattern**: GORM-based MySQL client with connection pool management, structured logging, and slow query detection.

**Key Components**:
- `Database` interface - Provides `DB()`, `Ping(ctx)`, and `Close()`
- `Config` struct - Configuration for connection, pool settings, and logging
- `gormLogger` - Custom GORM logger bridging GORM's logging interface with zap-based structured logging
- `NewMySQL(log, cfg)` factory - Creates and configures database connection with pool settings

**Architecture Details**:
- Built on `gorm.io/gorm` v2 for robust ORM capabilities
- Custom logger integrates GORM's logging with project's `logger.Logger` interface
- Connection pool managed by underlying `database/sql` with configurable parameters
- Slow query detection logs queries exceeding `SlowThreshold` at WARN level
- Prepared statements enabled by default for better performance and security
- Log levels: silent, error, warn, info
- DSN (Data Source Name) automatically constructed from config fields
- Initial `Ping()` test ensures connectivity during initialization

**Important Files**:
- `db.go` - Core `Database` interface
- `mysql.go` - MySQL implementation (`defaultMySQLDatabase`) and factory function
- `logger.go` - Custom GORM logger implementation with zap backend
- `config.go` - Configuration with validation and defaults
- `errors.go` - Error constructors and predefined errors

**Data Flow**:
1. User calls `NewMySQL(log, cfg)` with configuration
2. Configuration validated and merged with defaults
3. GORM DB instance created with custom logger
4. Connection pool settings applied (`MaxOpenConns`, `MaxIdleConns`, etc.)
5. Initial `Ping()` test verifies connectivity
6. User retrieves `*gorm.DB` via `DB()` for GORM operations
7. All SQL operations logged through custom logger with structured fields
8. User calls `Close()` to release connections gracefully

**Configuration**:
```go
cfg := &db.Config{
    Host:            "localhost",
    Port:            3306,
    User:            "root",
    Password:        "password",
    Database:        "myapp",
    MaxOpenConns:    25,              // Max open connections
    MaxIdleConns:    10,              // Max idle connections
    ConnMaxLifetime: 1800 * time.Second,  // 30 minutes
    ConnMaxIdleTime: 600 * time.Second,   // 10 minutes
    LogLevel:        "warn",          // "silent", "error", "warn", "info"
    SlowThreshold:   1 * time.Second, // Slow query threshold
    Charset:         "utf8mb4",
    Loc:             "Local",         // Timezone location
}
database, err := db.NewMySQL(log, cfg)
```

**Usage**:
```go
import (
    "github.com/dailyyoga/nexgo/db"
    "github.com/dailyyoga/nexgo/logger"
)

// Create logger
log, _ := logger.New(nil)
defer log.Sync()

// Create database client
database, err := db.NewMySQL(log, cfg)
if err != nil {
    log.Fatal("database connection failed", zap.Error(err))
}
defer database.Close()

// Health check with context
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
if err := database.Ping(ctx); err != nil {
    log.Error("database unhealthy", zap.Error(err))
}

// Get GORM instance for operations
gormDB, err := database.DB()
if err != nil {
    log.Error("failed to get db instance", zap.Error(err))
}

// Use GORM for CRUD operations
type User struct {
    ID   int64  `gorm:"primaryKey"`
    Name string `gorm:"size:100"`
    Age  int
}

gormDB.AutoMigrate(&User{})
gormDB.Create(&User{Name: "Alice", Age: 25})

var user User
gormDB.WithContext(ctx).First(&user, "name = ?", "Alice")
```

**Logging Examples**:
The custom logger provides structured logging for all SQL operations:
- **Normal Query (Info)**: `INFO sql trace component=gorm elapsed=2.3ms rows=5 sql="SELECT * FROM users"`
- **Slow Query (Warn)**: `WARN slow sql component=gorm elapsed=1.2s rows=1000 threshold=1s sql="SELECT * FROM users"`
- **SQL Error (Error)**: `ERROR sql error component=gorm elapsed=1.1ms error="Table 'mydb.invalid_table' doesn't exist"`


### Package: ch (ClickHouse Client)

**Core Pattern**: Unified client with shared connection for both query and async batch write operations.

**Key Components**:
- `Client` interface - Unified entry point providing `Writer()`, `Query()`, `QueryRow()`, and `Close()`
- `Writer` interface - Async batch writer with `Start()`, `Write()`, `Close()`, `RefreshTableSchema()`
- `Table` interface - Data models must implement `TableName()` and `ToValueMap()`
- `Converter` system - Automatic type conversion between Go types and ClickHouse types

**Architecture Details**:
- `defaultClient` manages a single `driver.Conn` shared by queries and writes (thread-safe)
- `Writer` uses unbounded channels (`chanx`) for non-blocking writes
- Schema discovery and caching: `DESCRIBE TABLE` results are cached in `tableSchemaCache`
- Schema refresh: Background goroutine periodically refreshes all cached table schemas (configurable via `SchemaRefreshInterval`)
- Manual schema refresh: Use `RefreshTableSchema(ctx, table)` to immediately update specific table schema
- Dual flush triggers: time-based (`FlushInterval`) and size-based (`FlushSize`)
- Type converters (`StringConverter`, `IntConverter`, etc.) handle automatic type coercion
- Non-insertable columns (MATERIALIZED, ALIAS, EPHEMERAL) are automatically filtered

**Important Files**:
- `ch.go` - Core interfaces
- `client.go` - Client implementation with connection management
- `writer.go` - Async batch writer with buffering and flushing logic
- `converter.go` - Type conversion system with `TableColumn` metadata parsing
- `config.go` - Configuration with validation

**Data Flow** (Write):
1. User calls `Write(rows)` → rows sent to unbounded channel
2. Background goroutine accumulates rows by table in local buffer
3. Flush triggered by size/time/shutdown
4. Schema fetched from cache or `DESCRIBE TABLE`
5. Batch insert via `conn.PrepareBatch()`

**Configuration**:
```go
cfg := &ch.Config{
    DSN:   "clickhouse://localhost:9000/default",
    Debug: false,
    Settings: clickhouse.Settings{
        "max_execution_time": 60,  // ClickHouse query settings
    },
    WriterConfig: &ch.WriterConfig{
        FlushInterval:         10 * time.Second, // Time-based flush check interval
        FlushSize:             5000,              // Size-based flush trigger (immediate flush)
        MinFlushSize:          500,               // Minimum batch size for time-triggered flush
        MaxWaitTime:           60 * time.Second,  // Maximum wait time to ensure data freshness
        SchemaRefreshInterval: 5 * time.Minute,   // Auto-refresh interval (0 = disabled)
    },
}
client, err := ch.NewClient(cfg, logger)
```

**Flush Strategy**:
The Writer uses a smart flush strategy to balance data freshness and ClickHouse best practices:
1. **Size trigger**: Flush immediately when `totalRows >= FlushSize` (default: 5000)
2. **Time trigger with MinFlushSize**: Flush on interval only when `totalRows >= MinFlushSize` (default: 500)
3. **MaxWaitTime guarantee**: Force flush when data has been waiting longer than `MaxWaitTime` (default: 60s)
4. **Shutdown flush**: Flush all remaining data regardless of size

This prevents frequent small batch writes during low traffic periods while ensuring data doesn't wait too long.

### Package: kafka (Kafka Consumer/Producer)

**Core Pattern**: Wrapper around confluent-kafka-go with automatic retry, parallel processing, and robust error handling.

#### Consumer

**Key Components**:
- `Consumer` interface - `Start(ctx, handler)` and `Close()`
- `ConsumerMsgHandler` - User-defined function `func(ctx, *Message) error`
- `defaultConsumer` - Manages multiple consumer instances for parallel processing

**Architecture Details**:
- Creates `InstanceNum` parallel consumer instances (goroutines)
- Each instance runs independent consumption loop with retry logic
- Manual offset commit after successful processing (unless `EnableAutoCommit=true`)
- Retry mechanism: up to `MaxRetries` attempts per message with exponential backoff
- Context cancellation propagates to all instances for graceful shutdown

**Data Flow** (Consumer):
1. `Start()` spawns `InstanceNum` goroutines
2. Each goroutine polls Kafka with `consumer.ReadMessage()`
3. On message: call handler with retry logic
4. On success: commit offset (if manual commit)
5. On context cancel: close all instances

#### Producer

**Key Components**:
- `Producer` interface - `Produce(ctx, msg)` and `Close()`
- `defaultProducer` - Manages producer instance with async delivery reports
- `Message` - Unified message type for both consumer and producer

**Architecture Details**:
- Single producer instance with async message delivery
- Background goroutine handles delivery reports from Events() channel
- Configurable batch sending via `LingerMs` and `BatchSize`
- Configurable compression (none, gzip, snappy, lz4, zstd)
- Configurable acks mechanism (0, 1, all) for reliability vs performance tradeoff
- Graceful shutdown: `Close()` flushes pending messages (10s timeout) before closing
- Auto-reconnection: producer creation retries on failure (3 attempts)

**Data Flow** (Producer):
1. User calls `Produce(ctx, msg)` → message queued by librdkafka
2. Background goroutine monitors `Events()` channel for delivery reports
3. On delivery: log success (Debug) or failure (Error)
4. On `ErrAllBrokersDown`: auto-close producer
5. On `Close()`: flush pending messages and wait for goroutine completion

**Important Files**:
- `kafka.go` - Core interfaces and types (shared by consumer and producer)
- `consumer.go` - Consumer factory and interface
- `consume.go` - Consumer implementation with retry loop
- `producer.go` - Producer factory and implementation
- `config.go` - Configuration for both consumer and producer with defaults
- `validate.go` - Kafka cluster validation (shared by consumer and producer)
- `errors.go` - Structured error types

### Package: cron (Cron Job Manager)

**Core Pattern**: Chain-based task execution with middleware pipeline and inter-task data sharing.

**Key Components**:
- `Cron` interface - `Start()`, `Close()`, `AddTasks(name, spec, ...tasks)`
- `Task` interface - `Name()` and `Run(ctx)`
- `Middleware` - `func(Task) Task` for wrapping tasks (Recovery, Logging)
- `SharedData` - Thread-safe `sync.Map` wrapper in context for inter-task communication

**Architecture Details**:
- Built on `robfig/cron/v3` with 6-field cron expressions (with seconds)
- `cronManager` wraps tasks in `chainJob` which executes tasks sequentially
- Middleware chain: Recovery → Logging → Custom (applied in order)
- Each chain execution gets fresh `SharedData` in context
- Chain aborts on first task error
- Graceful shutdown: `Close()` waits for running jobs via `cron.Stop().Wait()`

**Important Files**:
- `cron.go` - Core interfaces and `NewCron()` factory
- `manager.go` - `cronManager` and `chainJob` implementation
- `middleware.go` - Built-in middlewares (recovery, logging)
- `shared_data.go` - Thread-safe data sharing via `sync.Map`
- `errors.go` - Standard errors

**Data Flow** (Task Chain):
1. `AddTasks()` creates `chainJob` wrapping all tasks
2. On schedule trigger: create `SharedData` in context
3. For each task: apply middleware chain, execute `Run(ctx)`
4. If error: abort chain and log
5. If success: continue to next task
6. After all tasks: log completion

### Package: routine (Safe Goroutine Execution)

**Core Pattern**: Safe goroutine execution with automatic panic recovery to prevent application crashes.

**Key Components**:
- `Runner` interface - Provides `Go()`, `GoWithContext()`, `GoNamed()`, `GoNamedWithContext()`, `Wait()`
- Standalone functions - `Go()`, `GoWithContext()`, `GoNamed()`, `GoNamedWithContext()` for one-off usage
- `defaultRunner` - Implementation with `sync.WaitGroup` for coordinated shutdown

**Architecture Details**:
- Wraps all goroutine executions with `defer recover()` to catch panics
- Logs panics with stack traces using the `logger.Logger` interface
- `Runner` interface tracks goroutines via `sync.WaitGroup` for graceful shutdown
- Standalone functions provide simpler API when tracking is not needed
- Named variants allow identifying goroutines in logs for debugging

**Important Files**:
- `routine.go` - Core interfaces, Runner implementation, and standalone functions
- `errors.go` - Error types for panic recovery

**Usage** (Runner):
```go
import "github.com/dailyyoga/nexgo/routine"

runner := routine.New(log)

// Simple goroutine
runner.Go(func() {
    // work that might panic
})

// Named goroutine for better logging
runner.GoNamed("process-data", func() {
    // work
})

// With context
runner.GoWithContext(ctx, func(ctx context.Context) {
    // work with context
})

// Wait for all goroutines to complete
runner.Wait()
```

**Usage** (Standalone):
```go
// Simple one-off goroutine
routine.Go(log, func() {
    // work
})

// Named one-off goroutine
routine.GoNamed(log, "background-task", func() {
    // work
})
```

### Package: cache (Syncable Cache)

**Core Pattern**: Periodically syncing cache with automatic retry logic and exponential backoff.

**Key Components**:
- `SyncableCache[T]` interface - Generic cache with `Start()`, `Stop()`, `Get()`, `Sync(ctx)`
- `SyncFunc[T]` - User-defined function `func(ctx) (T, error)` to fetch data
- `SyncableCacheConfig` - Configuration for cache name, sync interval, timeout, and retry logic
- Generic implementation supports any data type T

**Architecture Details**:
- Built on Go generics for type-safe caching of any data structure
- Uses `sync.RWMutex` for thread-safe concurrent reads during sync operations
- Background goroutine performs periodic sync on configurable interval
- Exponential backoff retry with configurable max attempts (default: 3)
- Context-aware: respects timeout and cancellation for each sync operation
- Graceful shutdown: `Stop()` can be called multiple times safely with `sync.Once`
- Initial sync: `Start()` performs synchronous initial load before background sync begins
- Error classification: automatically distinguishes retryable (network, timeout) vs non-retryable errors
- Integration: uses `logger.Logger` for structured logging and `routine` package for safe goroutine execution
- **Reference Type Safety**: `Get()` returns a reference to cached data for reference types (slice, map, pointer). Callers must treat returned data as read-only to avoid data races. For value types (int, string, struct without pointers), values are automatically copied

**Important Files**:
- `cache.go` - Core interfaces and `SyncFunc[T]` type
- `syncable_cache.go` - Generic syncable cache implementation with retry logic
- `config.go` - Configuration with validation and defaults
- `errors.go` - Error constructors and predefined errors

**Data Flow** (Sync):
1. User creates cache with `NewSyncableCache(log, config, syncFunc)`
2. `Start()` performs initial sync synchronously (blocks until success or error)
3. Background goroutine starts with ticker for periodic sync
4. On each interval: call `syncFunc(ctx)` with timeout from config
5. On failure: retry with exponential backoff (1s, 2s, 4s, ...)
6. On success: update cache atomically under write lock
7. `Get()` returns current cached value under read lock (non-blocking)
8. `Stop()` cancels context to gracefully shutdown background goroutine

**Configuration**:
```go
cfg := &cache.SyncableCacheConfig{
    Name:         "my-cache",        // For logging/identification (required)
    SyncInterval: 5 * time.Minute,   // Periodic sync interval (default: 5m)
    SyncTimeout:  30 * time.Second,  // Timeout per sync attempt (default: 30s)
    MaxRetries:   3,                  // Max retry attempts on failure (default: 3)
}

cache, err := cache.NewSyncableCache(log, cfg, syncFunc)
```

**Usage**:
```go
import "github.com/dailyyoga/nexgo/cache"

// Define sync function to fetch data
syncFunc := func(ctx context.Context) ([]User, error) {
    // Fetch data from database, API, etc.
    return fetchUsersFromDB(ctx)
}

// Create cache
c, err := cache.NewSyncableCache(log, cfg, syncFunc)
if err != nil {
    return err
}

// Start periodic sync (blocks until initial sync succeeds)
if err := c.Start(); err != nil {
    return err
}
defer c.Stop()

// Get cached data (thread-safe, read-only for reference types)
users := c.Get()

// IMPORTANT: For reference types ([]User here), Get() returns a reference.
// ✅ Safe: Read-only access
for _, user := range users {
    fmt.Println(user.Name)  // OK
}

// ❌ Unsafe: Modifying returned data causes data races
// users[0].Name = "modified"  // DANGER!

// ✅ Safe: Create a copy if you need to modify
usersCopy := make([]User, len(users))
copy(usersCopy, users)
usersCopy[0].Name = "modified"  // OK

// Manually trigger sync if needed
if err := c.Sync(ctx); err != nil {
    log.Error("manual sync failed", zap.Error(err))
}
```

### Package: cache/Redis (Redis Client)

**Core Pattern**: Thin wrapper around go-redis v9 that embeds `redis.Cmdable` to automatically provide 200+ Redis commands.

**Key Components**:
- `Redis` interface - Embeds `redis.Cmdable` plus custom methods: `Subscribe()`, `PSubscribe()`, `Close()`, `Unwrap()`, `PoolStats()`
- `RedisConfig` struct - Configuration with connection pool settings and timeouts
- `NewRedis(log, cfg)` factory - Creates client, validates config, and tests connection

**Architecture Details**:
- Embeds `*redis.Client` to automatically implement all `redis.Cmdable` methods (200+ commands)
- Thread-safe: `redis.Client` handles all concurrency internally
- Connection test: `Ping()` executed during initialization to verify connectivity
- Custom Subscribe methods wait for subscription confirmation before returning
- Direct access to underlying client via `Unwrap()` for advanced operations (Pipeline, Transaction, etc.)
- Pool statistics available via `PoolStats()`

**Important Files**:
- `cache.go` - `Redis` interface definition (along with `SyncableCache`)
- `config.go` - `RedisConfig` with `Validate()`, `MergeDefaults()`, `Options()`
- `redis.go` - Implementation (`defaultRedis`) with `NewRedis()` factory
- `errors.go` - Redis-specific error constructors

**Configuration**:
```go
cfg := &cache.RedisConfig{
    Addr:            "localhost:6379",  // Redis address (required)
    Password:        "",                 // Auth password (default: "")
    DB:              0,                  // Database number (default: 0)
    PoolSize:        10,                 // Max connections (default: 10)
    MinIdleConns:    5,                  // Min idle connections (default: 5)
    MaxRetries:      3,                  // Max retries (default: 3)
    DialTimeout:     5 * time.Second,   // Dial timeout (default: 5s)
    ReadTimeout:     3 * time.Second,   // Read timeout (default: 3s)
    WriteTimeout:    3 * time.Second,   // Write timeout (default: 3s)
    ConnMaxIdleTime: 5 * time.Minute,   // Max idle time (default: 5m)
    ConnMaxLifetime: 0,                  // Max lifetime (default: 0, no limit)
}
rdb, err := cache.NewRedis(log, cfg)
```

**Usage**:
```go
import (
    "github.com/dailyyoga/nexgo/cache"
    "github.com/redis/go-redis/v9"
)

// Create Redis client
rdb, err := cache.NewRedis(log, cfg)
if err != nil {
    return err
}
defer rdb.Close()

ctx := context.Background()

// String operations
rdb.Set(ctx, "key", "value", time.Hour)
val, err := rdb.Get(ctx, "key").Result()
if err == cache.Nil {
    // Key does not exist
}

// Distributed lock with SetNX
ok, _ := rdb.SetNX(ctx, "lock:resource", "owner", 30*time.Second).Result()

// Hash operations
rdb.HSet(ctx, "user:1", "name", "Alice", "age", "25")
name, _ := rdb.HGet(ctx, "user:1", "name").Result()

// List operations
rdb.LPush(ctx, "queue", "task1", "task2")
task, _ := rdb.RPop(ctx, "queue").Result()

// Sorted set operations
rdb.ZAdd(ctx, "leaderboard", redis.Z{Score: 100, Member: "player1"})
rank, _ := rdb.ZRank(ctx, "leaderboard", "player1").Result()

// Pub/Sub
pubsub, err := rdb.Subscribe(ctx, "channel")
if err != nil {
    return err
}
defer pubsub.Close()

// Pattern subscription
pubsub, err := rdb.PSubscribe(ctx, "events:*")

// Lua script
result, _ := rdb.Eval(ctx, `return ARGV[1]`, nil, "hello").Result()

// Pipeline (via Unwrap)
pipe := rdb.Unwrap().Pipeline()
incr := pipe.Incr(ctx, "counter")
pipe.Expire(ctx, "counter", time.Hour)
pipe.Exec(ctx)
count, _ := incr.Result()

// Transaction (via Unwrap)
rdb.Unwrap().Watch(ctx, func(tx *redis.Tx) error {
    // Transaction logic
    return nil
}, "key")

// Pool statistics
stats := rdb.PoolStats()
log.Info("pool stats", zap.Uint32("total", stats.TotalConns))
```

**Available Commands** (via `redis.Cmdable`):
- **String**: Get, Set, SetNX, SetEX, MGet, MSet, Incr, Decr, Append, etc.
- **Key**: Del, Exists, Expire, TTL, Keys, Scan, Rename, Type, etc.
- **Hash**: HGet, HSet, HGetAll, HDel, HExists, HIncrBy, HScan, etc.
- **List**: LPush, RPush, LPop, RPop, LRange, LLen, LIndex, etc.
- **Set**: SAdd, SRem, SMembers, SIsMember, SCard, SInter, SUnion, etc.
- **Sorted Set**: ZAdd, ZRem, ZRange, ZRank, ZScore, ZCard, ZIncrBy, etc.
- **Script**: Eval, EvalSha, ScriptLoad, ScriptExists, ScriptFlush
- **Pub/Sub**: Publish (Subscribe/PSubscribe via custom methods)
- **Server**: Ping, Info, DBSize, FlushDB, etc.

## Cross-Cutting Patterns

### Logger Interface

The logger package provides a unified `Logger` interface used across all packages:

```go
// From github.com/dailyyoga/nexgo/logger
type Logger interface {
    Debug(msg string, fields ...zap.Field)
    Info(msg string, fields ...zap.Field)
    Warn(msg string, fields ...zap.Field)
    Error(msg string, fields ...zap.Field)
    Sync() error
}
```

**Key Points**:
- All packages (ch, kafka, cron, routine, cache) import and use `logger.Logger` interface
- Compatible with `*zap.Logger` - you can pass a zap logger directly
- Factory function `logger.New(cfg)` creates configured logger instances and sets the global logger
- Supports structured logging with zap fields
- Package-level functions (`logger.Info()`, etc.) provide convenient global access

**Usage Pattern**:
```go
import "github.com/dailyyoga/nexgo/logger"

// Create logger - also sets global logger automatically
log, err := logger.New(nil) // Uses default config
if err != nil {
    panic(err)
}
defer logger.Sync()

// Use with other packages (DI)
client, err := ch.NewClient(chConfig, log)
consumer, err := kafka.NewConsumer(log, kafkaConfig)
cronMgr := cron.NewCron(log)
runner := routine.New(log)
cache, err := cache.NewSyncableCache(log, cacheConfig, syncFunc)

// Or use global functions directly
logger.Info("server started", zap.String("addr", ":8080"))
logger.Error("request failed", zap.Error(err))
```

### Error Handling

All packages follow a unified error handling pattern, defining package-level errors in `errors.go`.

**Error Types**:

1. **Predefined Error Variables** - Package-level sentinel errors created using `fmt.Errorf`, suitable for errors that don't require additional context:
   ```go
   var ErrBufferFull = fmt.Errorf("ch: buffer is full")
   ```

2. **Error Constructor Functions** - Return new errors wrapping underlying errors, using `%w` format verb to support error chains:
   ```go
   func ErrConnection(err error) error {
       return fmt.Errorf("ch: connection failed: %w", err)
   }
   ```

**Error Definitions by Package**:

- **logger** package:
  - Error constructors: `ErrInvalidLevel(level, err)`, `ErrInvalidEncoding(encoding)`, `ErrBuildLogger(err)`

- **db** package:
  - Predefined errors: `ErrConnectionNotEstablished`
  - Error constructors: `ErrInvalidConfig(msg)`, `ErrConnection(err)`

- **ch** package:
  - Predefined errors: `ErrBufferFull`, `ErrWriterClosed`, `ErrConnectionClosed`, `ErrInvalidTable`
  - Error constructors: `ErrInvalidConfig(msg)`, `ErrConnection(err)`, `ErrInsert(tableName, err)`

- **kafka** package:
  - Predefined errors: `ErrNoConsumerInstances`
  - Error constructors: `ErrInvalidConfig(msg)`, `ErrConnection(err)`, `ErrSubscribe(topics, err)`, `ErrConsume(err)`, `ErrCommit(err)`

- **cron** package:
  - Predefined errors: `ErrNoTasks`, `ErrInvalidSpec`, `ErrCronClosed`

- **routine** package:
  - Predefined errors: `ErrPanicRecovered`
  - Error constructors: `ErrPanic(recovered)`

- **cache** package:
  - Predefined errors: `ErrCacheClosed`, `ErrInvalidConfig`
  - Error constructors: `ErrSync(err)`, `ErrInvalidName(name)`, `ErrInvalidSyncInterval(interval)`, `ErrInvalidSyncTimeout(timeout)`, `ErrInvalidMaxRetries(retries)`, `ErrInvalidRedisConfig(msg)`, `ErrRedisConnection(err)`, `ErrRedisOperation(op, err)`

**Error Checking**:
- Use `errors.Is()` to check predefined errors:
  ```go
  if errors.Is(err, ch.ErrBufferFull) {
      // handle buffer full
  }
  ```
- Use `errors.As()` to extract wrapped underlying errors (if needed)
- All error constructor functions use `%w` to wrap underlying errors, preserving the complete error chain for tracing

### Configuration Pattern
- Each package has `Config` struct with `Validate()` method
- Default configurations provided via `DefaultConfig()` functions
- `Validate()` performs validation only (does not modify config)
- `New()` functions merge default values before validation
- Required fields validated on construction

### Interface-Driven Design
- All major components exposed as interfaces for mocking in tests
- Implementations are private (`defaultClient`, `defaultConsumer`, `cronManager`)
- Factory functions return interfaces (`NewClient()`, `NewConsumer()`, `NewProducer()`, `NewCron()`)
- Logger interface from logger package used consistently across all packages

## Important Notes

- **Logger Package**: `logger.New(cfg)` creates a logger for DI and automatically sets the global logger. Use `logger.Info()`, `logger.Error()` etc. for convenient global access. Both DI and global functions use the same configuration.
- **Global Logger**: Concurrency-safe, lazy-initialized with default config if `New()` is never called. `CallerSkip` is handled automatically for correct caller information.
- **Logger Interface**: All packages use `logger.Logger` interface - compatible with `*zap.Logger`
- **Database Package**: Must call `Close()` on shutdown to release connections. Use `Ping(ctx)` for health checks. Connection pool settings should be tuned based on expected load.
- **Database Logging**: Set `LogLevel: "warn"` with `SlowThreshold` to monitor slow queries. Use `LogLevel: "info"` for full SQL tracing in development.
- **GORM Usage**: Always use `WithContext(ctx)` for query operations to support cancellation and timeout. Check `result.Error` after all GORM operations.
- **ClickHouse Writer**: Must call `Start()` before `Write()`, call `Close()` to flush pending data
- **Kafka Consumer**: Context cancellation is the primary shutdown mechanism
- **Kafka Producer**: Must call `Close()` to flush pending messages (10s timeout). Delivery reports are logged asynchronously
- **Cron Tasks**: Return errors from `Run()`, never panic (recovery middleware will catch)
- **Schema Changes**: ClickHouse schema cache auto-refreshes by default (every 5 minutes). For immediate effect after DDL changes, call `writer.RefreshTableSchema(ctx, tableName)` or set `SchemaRefreshInterval: 0` to disable auto-refresh
- **Type Conversion**: Int64 timestamps are assumed to be milliseconds, converted to `time.Time` automatically
- **Enum Handling**: Empty strings in enum columns replaced with first enum value
- **Routine Package**: Use `Runner` interface when you need to track goroutines and wait for completion; use standalone functions for fire-and-forget scenarios
- **Cache Package**: Must call `Start()` before `Get()`, `Start()` blocks until initial sync succeeds. Call `Stop()` for graceful shutdown. Sync errors are retried automatically with exponential backoff. **CRITICAL**: `Get()` returns a reference for reference types (slice, map, pointer) - treat returned data as read-only to avoid data races. Create a deep copy if modification is needed. Value types (int, string, struct without pointers) are automatically safe as Go copies them
- **Redis Package**: Call `Close()` on shutdown. Use `cache.Nil` to check for non-existent keys. Use `Unwrap()` for Pipeline/Transaction operations. The client is fully thread-safe