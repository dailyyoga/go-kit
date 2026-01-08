# nexgo

A collection of Go utility packages for common infrastructure patterns.

English | [简体中文](./README.zh-CN.md)

## Packages

### [logger](./logger) - Unified Logging Interface

Unified logging interface based on zap with configurable levels, encoding, and output paths.

- Standard Logger interface compatible with *zap.Logger
- Configurable log levels (debug, info, warn, error, etc.)
- Multiple encoding formats (JSON, Console)
- Flexible output configuration
- Used consistently across all packages

### [db](./db) - MySQL Database Client

GORM-based MySQL database client with connection pool management and structured logging.

- Built on GORM v2 for robust ORM capabilities
- Connection pool management with configurable settings
- Structured logging with slow query detection
- Custom logger integration with zap backend
- Prepared statements enabled by default
- Context-aware operations for cancellation and timeout

[View documentation →](./db/README.md)

### [ch](./ch) - ClickHouse Client

Unified ClickHouse client with support for both query operations and asynchronous batch writes.

- Shared connection for queries and writes
- Async batch writes with dual flush triggers (time and size)
- Automatic type conversion
- Schema discovery and caching

[View documentation →](./ch/README.md)

### [kafka](./kafka) - Kafka Consumer/Producer

Lightweight Kafka consumer and producer wrapper built on confluent-kafka-go.

- Simple and intuitive API
- Consumer: Multiple instances for parallel processing with automatic retry
- Producer: Async delivery with configurable batching and compression
- Flexible offset management and reliability configurations

[View documentation →](./kafka/README.md)

### [cron](./cron) - Cron Job Manager

High-performance cron job manager with chain-based task execution and middleware support.

- Chain-based sequential execution
- Middleware support (recovery, logging)
- Thread-safe inter-task data sharing
- Automatic panic recovery

[View documentation →](./cron/README.md)

### [routine](./routine) - Safe Goroutine Execution

Safe goroutine execution with automatic panic recovery to prevent application crashes.

- Automatic panic recovery with stack trace logging
- Runner interface for tracking and waiting on goroutines
- Standalone functions for fire-and-forget scenarios
- Named goroutines for better debugging and logging

### [cache](./cache) - Syncable Cache & Redis Client

Periodically syncing cache with automatic retry logic, and Redis client wrapper.

**SyncableCache:**
- Generic cache supporting any data type with Go generics
- Periodic background synchronization from configurable data source
- Automatic retry with exponential backoff for transient failures
- Thread-safe concurrent reads during sync operations
- Context-aware with configurable timeout per sync
- **Reference Type Safety**: For slice, map, pointer types, `Get()` returns a reference - treat as read-only to avoid data races

**Redis:**
- Thin wrapper around go-redis v9 with 200+ commands via `redis.Cmdable`
- Connection pool management with configurable settings
- Thread-safe operations
- Pub/Sub support with subscription confirmation
- Pool statistics monitoring

[View documentation →](./cache/README.md)

## Installation

```bash
go get github.com/dailyyoga/nexgo
```

## Quick Start

### Logger

**Default configuration:**
```go
import "github.com/dailyyoga/nexgo/logger"

// Use default configuration (info level, JSON encoding, stdout)
log, _ := logger.New(nil)
defer log.Sync()

log.Info("application started")
```

**Custom configuration:**
```go
import "github.com/dailyyoga/nexgo/logger"

cfg := &logger.Config{
    Level:            "debug",
    Encoding:         "console",
    OutputPaths:      []string{"stdout", "/var/log/app.log"},
    ErrorOutputPaths: []string{"stderr"},
}
log, _ := logger.New(cfg)
defer log.Sync()
```

### Database

**Basic Usage:**
```go
import (
    "github.com/dailyyoga/nexgo/db"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
cfg := &db.Config{
    Host:     "localhost",
    Port:     3306,
    User:     "root",
    Password: "password",
    Database: "myapp",
}
database, _ := db.NewMySQL(log, cfg)
defer database.Close()

// Get GORM instance
gormDB, _ := database.DB()

// CRUD operations
type User struct {
    ID   int64  `gorm:"primaryKey"`
    Name string `gorm:"size:100"`
    Age  int
}

gormDB.AutoMigrate(&User{})
gormDB.Create(&User{Name: "Alice", Age: 25})

// Query with context
ctx := context.Background()
var user User
gormDB.WithContext(ctx).First(&user, "name = ?", "Alice")
```

**Health Check:**
```go
// Ping database with timeout
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

if err := database.Ping(ctx); err != nil {
    log.Error("database unhealthy", zap.Error(err))
}
```

### ClickHouse

```go
import (
    "github.com/dailyyoga/nexgo/ch"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
client, _ := ch.NewClient(config, log)
defer client.Close()

writer, _ := client.Writer()
writer.Start()
writer.Write(ctx, events)
```

### Kafka

**Consumer:**
```go
import (
    "github.com/dailyyoga/nexgo/kafka"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
consumer, _ := kafka.NewConsumer(log, config)
defer consumer.Close()

consumer.Start(ctx, func(ctx context.Context, msg *kafka.Message) error {
    // Process message
    return nil
})
```

**Producer:**
```go
import (
    "github.com/dailyyoga/nexgo/kafka"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
producer, _ := kafka.NewProducer(log, config)
defer producer.Close()

topic := "my-topic"
producer.Produce(ctx, &kafka.Message{
    TopicPartition: kafka.TopicPartition{Topic: &topic},
    Value:          []byte("message"),
})
```

### Cron

```go
import (
    "github.com/dailyyoga/nexgo/cron"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
manager := cron.NewCron(log)
manager.AddTasks("job-name", "0 0 * * * *", task1, task2)
manager.Start()
defer manager.Close()
```

### Routine

**Using Runner (track and wait):**
```go
import (
    "github.com/dailyyoga/nexgo/routine"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
runner := routine.New(log)

runner.GoNamed("process-data", func() {
    // Work that might panic - will be safely recovered
})

runner.GoWithContext(ctx, func(ctx context.Context) {
    // Work with context
})

runner.Wait() // Wait for all goroutines to complete
```

**Standalone functions (fire-and-forget):**
```go
import (
    "github.com/dailyyoga/nexgo/routine"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)

// Simple one-off goroutine
routine.Go(log, func() {
    // Work - panic will be recovered and logged
})

// Named for better logging
routine.GoNamed(log, "background-task", func() {
    // Work
})
```

### Cache

```go
import (
    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)

// Define sync function
syncFunc := func(ctx context.Context) ([]User, error) {
    return fetchUsersFromDB(ctx)
}

// Create and start cache
cfg := &cache.SyncableCacheConfig{
    Name:         "user-cache",        // Required field
    SyncInterval: 5 * time.Minute,
    SyncTimeout:  30 * time.Second,
    MaxRetries:   3,
}
c, _ := cache.NewSyncableCache(log, cfg, syncFunc)
c.Start() // Blocks until initial sync succeeds
defer c.Stop()

// Get cached data (thread-safe, read-only)
users := c.Get()

// IMPORTANT: For reference types (slice, map, pointer), Get() returns
// a reference to cached data. Treat it as read-only:
// ✅ OK: for _, u := range users { fmt.Println(u.Name) }
// ❌ DANGER: users[0].Name = "modified"  // Data race!
// If you need to modify, create a copy first
```

### Redis

```go
import (
    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
    "github.com/redis/go-redis/v9"
)

log, _ := logger.New(nil)

// Create Redis client
cfg := &cache.RedisConfig{
    Addr:     "localhost:6379",
    PoolSize: 10,
}
rdb, _ := cache.NewRedis(log, cfg)
defer rdb.Close()

ctx := context.Background()

// String operations
rdb.Set(ctx, "key", "value", time.Hour)
val, _ := rdb.Get(ctx, "key").Result()

// Check if key exists
if err == cache.Nil {
    // Key does not exist
}

// Hash operations
rdb.HSet(ctx, "user:1", "name", "Alice", "age", "25")

// Pub/Sub
pubsub, _ := rdb.Subscribe(ctx, "channel")
defer pubsub.Close()

// Pipeline (via Unwrap)
pipe := rdb.Unwrap().Pipeline()
pipe.Incr(ctx, "counter")
pipe.Expire(ctx, "counter", time.Hour)
pipe.Exec(ctx)
```

## Dependencies

- Go 1.24.6+
- [ClickHouse/clickhouse-go/v2](https://github.com/ClickHouse/clickhouse-go)
- [confluentinc/confluent-kafka-go/v2](https://github.com/confluentinc/confluent-kafka-go)
- [redis/go-redis/v9](https://github.com/redis/go-redis)
- [robfig/cron/v3](https://github.com/robfig/cron)
- [uber-go/zap](https://github.com/uber-go/zap)

## Development

```bash
# Run tests
go test ./...

# Format code
go fmt ./...

# Check code
go vet ./...
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
