# cache

A Go cache library providing periodic synchronization cache and Redis client wrapper.

## Features

### SyncableCache
- **Generic Type Support**: Built with Go generics to support any data type in a type-safe manner
- **Periodic Synchronization**: Automatically refreshes cached data from a configurable source at regular intervals
- **Automatic Retry**: Exponential backoff retry mechanism for transient failures (network issues, timeouts)
- **Thread-Safe**: Concurrent reads are safe during sync operations using `sync.RWMutex`
- **Context-Aware**: Respects context timeout and cancellation for each sync operation
- **Graceful Shutdown**: Safe shutdown with `Stop()` that can be called multiple times
- **Initial Sync**: Blocks on `Start()` until initial data is successfully loaded
- **Error Classification**: Automatically distinguishes retryable vs non-retryable errors

### Redis
- **200+ Commands**: Embeds `redis.Cmdable` to provide all Redis commands automatically
- **Connection Pool**: Configurable connection pool management
- **Thread-Safe**: All operations are thread-safe via go-redis
- **Pub/Sub Support**: Subscribe methods wait for confirmation before returning
- **Pool Statistics**: Monitor connection pool health via `PoolStats()`

## Installation

```bash
go get github.com/dailyyoga/nexgo/cache
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
)

type User struct {
    ID   int64
    Name string
}

func main() {
    // Create logger
    log, err := logger.New(nil)
    if err != nil {
        panic(err)
    }
    defer log.Sync()

    // Define sync function to fetch data
    syncFunc := func(ctx context.Context) ([]User, error) {
        // Fetch from database, API, etc.
        return fetchUsersFromDB(ctx)
    }

    // Configure cache
    cfg := &cache.SyncableCacheConfig{
        Name:         "user-cache",
        SyncInterval: 5 * time.Minute,   // Sync every 5 minutes
        SyncTimeout:  30 * time.Second,  // 30s timeout per sync
        MaxRetries:   3,                  // Retry up to 3 times
    }

    // Create cache
    c, err := cache.NewSyncableCache(log, cfg, syncFunc)
    if err != nil {
        log.Fatal(err)
    }

    // Start periodic sync (blocks until initial sync succeeds)
    if err := c.Start(); err != nil {
        log.Fatal("initial sync failed:", err)
    }
    defer c.Stop()

    // Get cached data (thread-safe, non-blocking)
    users := c.Get()
    log.Printf("Loaded %d users", len(users))

    // Manually trigger sync if needed
    if err := c.Sync(context.Background()); err != nil {
        log.Printf("manual sync failed: %v", err)
    }
}

func fetchUsersFromDB(ctx context.Context) ([]User, error) {
    // Your implementation here
    return []User{
        {ID: 1, Name: "Alice"},
        {ID: 2, Name: "Bob"},
    }, nil
}
```

### Advanced Usage with Custom Type

```go
type ConfigData struct {
    Settings   map[string]string
    FeatureFlags map[string]bool
    UpdatedAt  time.Time
}

// Sync function fetches config from remote API
syncFunc := func(ctx context.Context) (*ConfigData, error) {
    resp, err := http.Get("https://api.example.com/config")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var config ConfigData
    if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
        return nil, err
    }

    config.UpdatedAt = time.Now()
    return &config, nil
}

// Create cache with pointer type
cfg := &cache.SyncableCacheConfig{
    Name:         "app-config",
    SyncInterval: 1 * time.Minute,
    SyncTimeout:  10 * time.Second,
    MaxRetries:   5,
}

configCache, _ := cache.NewSyncableCache(logger, cfg, syncFunc)
configCache.Start()
defer configCache.Stop()

// Get config (returns pointer)
config := configCache.Get()
if config.FeatureFlags["new_feature"] {
    // Use feature
}
```

## Configuration

### Config Structure

```go
type SyncableCacheConfig struct {
    // Name is used for logging purposes to identify the cache
    // Required: must be explicitly set by the user
    Name string

    // SyncInterval is the interval between periodic sync operations
    // Default: 5 * time.Minute
    SyncInterval time.Duration

    // SyncTimeout is the timeout for each sync operation
    // Default: 30 * time.Second
    SyncTimeout time.Duration

    // MaxRetries is the maximum number of retry attempts for failed sync operations
    // Default: 3
    MaxRetries int
}
```

### Default Configuration

```go
// Name field is required, other fields will use defaults
cfg := &cache.SyncableCacheConfig{
    Name: "my-cache",  // Required field
    // SyncInterval, SyncTimeout, MaxRetries will use defaults
}
cache, err := cache.NewSyncableCache(log, cfg, syncFunc)

// Passing nil config will fail validation because Name is required
// cache, err := cache.NewSyncableCache(log, nil, syncFunc)  // Error: Name is required
```

## Architecture

### Core Components

#### SyncableCache Interface

```go
type SyncableCache[T any] interface {
    // Start begins the periodic sync process
    // Performs an initial sync before starting the background goroutine
    // Returns error if initial sync fails
    Start() error

    // Stop gracefully stops the periodic sync process
    // Can be called multiple times safely
    Stop()

    // Get returns the current cached value
    // Safe to call concurrently with sync operations
    Get() T

    // Sync manually triggers a sync operation
    // Returns error if sync fails after all retry attempts
    Sync(ctx context.Context) error
}
```

#### SyncFunc

```go
// SyncFunc is a function that performs the actual sync operation
// Should return the new cache data or an error
// Must respect the context for cancellation and timeout
type SyncFunc[T any] func(ctx context.Context) (T, error)
```

### Retry Logic

The cache implements exponential backoff for retry:

1. **First attempt**: Immediate sync
2. **Second attempt**: 1 second backoff
3. **Third attempt**: 2 seconds backoff
4. **Fourth attempt**: 4 seconds backoff
5. And so on...

**Retryable errors** (automatically detected):
- Context deadline exceeded
- Connection refused/reset
- Broken pipe
- Timeout
- Too many connections
- Temporary failure
- Network unreachable

**Non-retryable errors** will fail immediately without retry.

### Data Flow

1. User calls `NewSyncableCache(log, config, syncFunc)`
2. `Start()` is called:
   - Performs initial sync synchronously (blocks until success)
   - Starts background goroutine for periodic sync
3. Background goroutine:
   - Waits for `SyncInterval` ticker
   - Calls `syncFunc(ctx)` with `SyncTimeout`
   - On failure: retries with exponential backoff up to `MaxRetries`
   - On success: updates cache atomically under write lock
4. `Get()` returns current cached value (uses read lock, non-blocking)
5. `Sync(ctx)` can manually trigger sync at any time
6. `Stop()` cancels context to gracefully shutdown background goroutine

## Error Handling

### Predefined Errors

```go
var (
    // ErrCacheClosed is returned when operations are attempted on a closed cache
    ErrCacheClosed = fmt.Errorf("cache: cache is closed")

    // ErrInvalidConfig is returned when the configuration is invalid
    ErrInvalidConfig = fmt.Errorf("cache: invalid config")
)
```

### Error Constructors

```go
// ErrSync wraps a sync operation error
func ErrSync(err error) error

// ErrInvalidName returns an error for invalid name
func ErrInvalidName(name string) error

// ErrInvalidSyncInterval returns an error for invalid sync interval
func ErrInvalidSyncInterval(interval time.Duration) error

// ErrInvalidSyncTimeout returns an error for invalid sync timeout
func ErrInvalidSyncTimeout(timeout time.Duration) error

// ErrInvalidMaxRetries returns an error for invalid max retries
func ErrInvalidMaxRetries(retries int) error
```

### Error Checking

```go
import "errors"

if err := cache.Start(); err != nil {
    if errors.Is(err, cache.ErrInvalidConfig) {
        // Handle configuration error
    }
    // Check wrapped error
    var syncErr error
    if errors.As(err, &syncErr) {
        // Handle specific sync error
    }
}
```

## Best Practices

### 1. Choose Appropriate Sync Interval

```go
// For frequently changing data
cfg := &cache.SyncableCacheConfig{
    SyncInterval: 30 * time.Second,  // Sync every 30 seconds
}

// For relatively stable data
cfg := &cache.SyncableCacheConfig{
    SyncInterval: 10 * time.Minute,  // Sync every 10 minutes
}
```

### 2. Set Reasonable Timeout

```go
// For fast local operations
cfg := &cache.SyncableCacheConfig{
    SyncTimeout: 5 * time.Second,
}

// For remote API calls
cfg := &cache.SyncableCacheConfig{
    SyncTimeout: 30 * time.Second,
}
```

### 3. Handle Initial Sync Failure

```go
// Start blocks until initial sync succeeds
if err := cache.Start(); err != nil {
    // Use fallback data or fail fast
    log.Fatal("cannot start without initial data:", err)
}
```

### 4. Use Pointer Types for Large Data

```go
// For large data structures, use pointer to avoid copying
type LargeData struct {
    Items []Item  // Could be large
}

syncFunc := func(ctx context.Context) (*LargeData, error) {
    return &LargeData{Items: items}, nil
}

cache, _ := cache.NewSyncableCache[*LargeData](log, cfg, syncFunc)
```

### 5. Graceful Shutdown

```go
// Use defer to ensure Stop is called
cache, err := cache.NewSyncableCache(log, cfg, syncFunc)
if err != nil {
    return err
}
defer cache.Stop()  // Always cleanup

cache.Start()
// ... use cache ...
```

### 6. Manual Sync When Needed

```go
// Trigger immediate sync (e.g., after external update)
if err := cache.Sync(ctx); err != nil {
    log.Error("manual sync failed", zap.Error(err))
    // Continue with stale data or retry
}
```

### 7. Treat Cached Data as Read-Only (Reference Types)

**Critical for Concurrent Safety**: When using reference types (slice, map, pointer, chan), `Get()` returns a reference to the internal cache data, not a deep copy. You **MUST** treat the returned value as read-only.

#### Safe Usage (Read-Only Access)

```go
// ✅ Safe - read-only access
users := cache.Get()  // T = []User
for _, user := range users {
    fmt.Println(user.Name)  // OK
}

// ✅ Safe - passing to read-only function
users := cache.Get()
displayUsers(users)  // OK if displayUsers doesn't modify
```

#### Unsafe Usage (Modifying Returned Data)

```go
// ❌ DANGEROUS - data race!
users := cache.Get()  // T = []User
users[0].Name = "modified"  // Modifies shared cache data

// ❌ DANGEROUS - appending to slice
users := cache.Get()  // T = []User
users = append(users, newUser)  // May modify shared backing array

// ❌ DANGEROUS - modifying map
config := cache.Get()  // T = map[string]string
config["key"] = "value"  // Modifies shared cache data
```

#### Solution: Create a Deep Copy When Modification is Needed

```go
// ✅ Safe - create a copy before modifying
users := cache.Get()  // T = []User
usersCopy := make([]User, len(users))
copy(usersCopy, users)
usersCopy[0].Name = "modified"  // OK - modifying copy

// ✅ Safe - copy map before modifying
config := cache.Get()  // T = map[string]string
configCopy := make(map[string]string, len(config))
for k, v := range config {
    configCopy[k] = v
}
configCopy["key"] = "value"  // OK - modifying copy

// ✅ Safe - copy pointer data
data := cache.Get()  // T = *ConfigData
dataCopy := *data  // Shallow copy
dataCopy.Setting = "modified"  // OK if Setting is value type
```

#### Value Types Don't Have This Issue

```go
// ✅ Automatically safe - value types are copied
count := cache.Get()  // T = int
count++  // OK - modifying local copy

text := cache.Get()  // T = string
text = text + " modified"  // OK - strings are immutable

// ✅ Safe - struct with value fields only
type Config struct {
    Port int
    Host string
}
cfg := cache.Get()  // T = Config (not *Config)
cfg.Port = 8080  // OK - modifying local copy
```

#### Design Patterns for Safe Usage

**Pattern 1: Read-Only Access (Recommended)**
```go
// Design your cache for read-only access
type UserCache struct {
    cache cache.SyncableCache[[]User]
}

func (uc *UserCache) GetUser(id int64) (User, bool) {
    users := uc.cache.Get()
    for _, user := range users {  // Read-only iteration
        if user.ID == id {
            return user, true  // Returns a copy
        }
    }
    return User{}, false
}
```

**Pattern 2: Copy-on-Read**
```go
// Provide a method that returns a copy
type UserCache struct {
    cache cache.SyncableCache[[]User]
}

func (uc *UserCache) GetUsersCopy() []User {
    users := uc.cache.Get()
    usersCopy := make([]User, len(users))
    copy(usersCopy, users)
    return usersCopy
}

// Callers can safely modify the copy
users := userCache.GetUsersCopy()
users[0].Name = "modified"  // OK
```

**Pattern 3: Use Value Types When Possible**
```go
// Instead of []User, use map[int64]User (value type values)
type UserMap map[int64]User

cache, _ := cache.NewSyncableCache[UserMap](log, cfg, syncFunc)

// Still need to copy map, but values are copied automatically
users := cache.Get()
usersCopy := make(UserMap, len(users))
for k, v := range users {
    usersCopy[k] = v  // v is copied (value type)
}
```

## Use Cases

### 1. Configuration Cache

Cache application configuration from remote config service:

```go
type AppConfig struct {
    DatabaseURL string
    APIKeys     map[string]string
    Features    []string
}

syncFunc := func(ctx context.Context) (*AppConfig, error) {
    return fetchConfigFromConsul(ctx)
}

configCache, _ := cache.NewSyncableCache(log, &cache.SyncableCacheConfig{
    Name:         "app-config",
    SyncInterval: 1 * time.Minute,
}, syncFunc)
```

### 2. User Permission Cache

Cache user permissions from database:

```go
type UserPermissions map[int64][]string  // userID -> permissions

syncFunc := func(ctx context.Context) (UserPermissions, error) {
    return db.FetchAllUserPermissions(ctx)
}

permCache, _ := cache.NewSyncableCache(log, &cache.SyncableCacheConfig{
    Name:         "permissions",
    SyncInterval: 5 * time.Minute,
}, syncFunc)

// Check permission
perms := permCache.Get()
if hasPermission(perms[userID], "admin") {
    // Allow action
}
```

### 3. Feature Flag Cache

Cache feature flags from feature management service:

```go
type FeatureFlags map[string]bool

syncFunc := func(ctx context.Context) (FeatureFlags, error) {
    return featureService.GetAllFlags(ctx)
}

flagCache, _ := cache.NewSyncableCache(log, &cache.SyncableCacheConfig{
    Name:         "feature-flags",
    SyncInterval: 30 * time.Second,
}, syncFunc)

// Check flag
if flagCache.Get()["new_checkout_flow"] {
    // Use new flow
}
```

### 4. Reference Data Cache

Cache reference data (countries, currencies, etc.):

```go
type ReferenceData struct {
    Countries  []Country
    Currencies []Currency
    Timezones  []Timezone
}

syncFunc := func(ctx context.Context) (*ReferenceData, error) {
    return loadReferenceData(ctx)
}

refCache, _ := cache.NewSyncableCache(log, &cache.SyncableCacheConfig{
    Name:         "reference-data",
    SyncInterval: 1 * time.Hour,  // Rarely changes
    MaxRetries:   5,
}, syncFunc)
```

## Testing

### Mock SyncFunc for Testing

```go
func TestCache(t *testing.T) {
    log, _ := logger.New(nil)

    // Mock sync function
    callCount := 0
    syncFunc := func(ctx context.Context) ([]string, error) {
        callCount++
        return []string{"item1", "item2"}, nil
    }

    cfg := &cache.SyncableCacheConfig{
        SyncInterval: 100 * time.Millisecond,
        SyncTimeout:  1 * time.Second,
    }

    c, err := cache.NewSyncableCache(log, cfg, syncFunc)
    require.NoError(t, err)

    err = c.Start()
    require.NoError(t, err)
    defer c.Stop()

    // Initial sync should have been called
    assert.Equal(t, 1, callCount)

    // Get cached data
    items := c.Get()
    assert.Equal(t, []string{"item1", "item2"}, items)

    // Wait for periodic sync
    time.Sleep(150 * time.Millisecond)
    assert.GreaterOrEqual(t, callCount, 2)
}
```

### Test Retry Logic

```go
func TestCacheRetry(t *testing.T) {
    log, _ := logger.New(nil)

    attempts := 0
    syncFunc := func(ctx context.Context) (string, error) {
        attempts++
        if attempts < 3 {
            return "", fmt.Errorf("temporary failure")
        }
        return "success", nil
    }

    cfg := &cache.SyncableCacheConfig{
        MaxRetries: 5,
    }

    c, _ := cache.NewSyncableCache(log, cfg, syncFunc)
    err := c.Start()

    // Should succeed after retries
    assert.NoError(t, err)
    assert.GreaterOrEqual(t, attempts, 3)
    assert.Equal(t, "success", c.Get())
}
```

## Thread Safety

The cache is fully thread-safe:

- **Read operations** (`Get()`): Multiple goroutines can read concurrently
- **Write operations** (during sync): Protected by write lock, blocks reads briefly
- **Control operations** (`Start()`, `Stop()`): Safe to call from multiple goroutines

```go
// Safe to use from multiple goroutines
go func() {
    for {
        data := cache.Get()  // Thread-safe read
        processData(data)
        time.Sleep(time.Second)
    }
}()

go func() {
    for {
        cache.Sync(ctx)  // Thread-safe manual sync
        time.Sleep(5 * time.Second)
    }
}()
```

## Performance Considerations

1. **Read Performance**: `Get()` is very fast (only read lock, no allocation)
2. **Write Performance**: Sync blocks reads briefly during data update
3. **Memory**: Cache stores complete copy of data in memory
4. **Sync Frequency**: Balance freshness vs system load

```go
// For read-heavy workloads with large data
// - Use longer sync intervals
// - Use pointer types to avoid copying
// - Consider data size in memory

cfg := &cache.SyncableCacheConfig{
    SyncInterval: 10 * time.Minute,  // Less frequent sync
}

// For write-heavy sources with smaller data
// - Use shorter sync intervals
// - Value types are fine

cfg := &cache.SyncableCacheConfig{
    SyncInterval: 30 * time.Second,  // More frequent sync
}
```

---

## Redis Client

### Quick Start

```go
package main

import (
    "context"
    "time"

    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
    "github.com/redis/go-redis/v9"
)

func main() {
    log, _ := logger.New(nil)
    defer log.Sync()

    // Create Redis client
    cfg := &cache.RedisConfig{
        Addr:     "localhost:6379",
        Password: "",        // No password
        DB:       0,         // Default DB
        PoolSize: 10,        // Connection pool size
    }

    rdb, err := cache.NewRedis(log, cfg)
    if err != nil {
        panic(err)
    }
    defer rdb.Close()

    ctx := context.Background()

    // String operations
    rdb.Set(ctx, "key", "value", time.Hour)
    val, err := rdb.Get(ctx, "key").Result()
    if err == cache.Nil {
        // Key does not exist
    }

    // Hash operations
    rdb.HSet(ctx, "user:1", "name", "Alice", "age", "25")
    name, _ := rdb.HGet(ctx, "user:1", "name").Result()

    // List operations
    rdb.LPush(ctx, "queue", "task1", "task2")
    task, _ := rdb.RPop(ctx, "queue").Result()

    log.Info("Redis operations completed")
}
```

### Configuration

```go
type RedisConfig struct {
    Addr            string        // Redis address (default: localhost:6379)
    Password        string        // Password for auth (default: "")
    DB              int           // Database number (default: 0)
    PoolSize        int           // Max connections (default: 10)
    MinIdleConns    int           // Min idle connections (default: 5)
    MaxRetries      int           // Max retries (default: 3)
    DialTimeout     time.Duration // Dial timeout (default: 5s)
    ReadTimeout     time.Duration // Read timeout (default: 3s)
    WriteTimeout    time.Duration // Write timeout (default: 3s)
    ConnMaxIdleTime time.Duration // Max idle time (default: 5m)
    ConnMaxLifetime time.Duration // Max lifetime (default: 0, no limit)
}
```

### Interface

```go
type Redis interface {
    redis.Cmdable  // All 200+ Redis commands

    // Subscribe subscribes to channels and waits for confirmation
    Subscribe(ctx context.Context, channels ...string) (*redis.PubSub, error)

    // PSubscribe subscribes to channel patterns and waits for confirmation
    PSubscribe(ctx context.Context, patterns ...string) (*redis.PubSub, error)

    // Close closes the client connection
    Close() error

    // Unwrap returns the underlying redis.Client for advanced operations
    Unwrap() *redis.Client

    // PoolStats returns connection pool statistics
    PoolStats() *redis.PoolStats
}
```

### Available Commands

All commands from `redis.Cmdable` are available:

| Category | Commands |
|----------|----------|
| **String** | Get, Set, SetNX, SetEX, MGet, MSet, Incr, Decr, Append, etc. |
| **Key** | Del, Exists, Expire, TTL, Keys, Scan, Rename, Type, etc. |
| **Hash** | HGet, HSet, HGetAll, HDel, HExists, HIncrBy, HScan, etc. |
| **List** | LPush, RPush, LPop, RPop, LRange, LLen, LIndex, etc. |
| **Set** | SAdd, SRem, SMembers, SIsMember, SCard, SInter, SUnion, etc. |
| **Sorted Set** | ZAdd, ZRem, ZRange, ZRank, ZScore, ZCard, ZIncrBy, etc. |
| **Script** | Eval, EvalSha, ScriptLoad, ScriptExists, ScriptFlush |
| **Pub/Sub** | Publish (Subscribe/PSubscribe via custom methods) |
| **Server** | Ping, Info, DBSize, FlushDB, etc. |

### Usage Examples

#### Distributed Lock

```go
// Acquire lock
ok, _ := rdb.SetNX(ctx, "lock:resource", "owner-id", 30*time.Second).Result()
if ok {
    defer rdb.Del(ctx, "lock:resource")
    // Do work...
}
```

#### Pub/Sub

```go
// Subscribe to channel
pubsub, err := rdb.Subscribe(ctx, "notifications")
if err != nil {
    return err
}
defer pubsub.Close()

// Receive messages
for msg := range pubsub.Channel() {
    fmt.Printf("Received: %s\n", msg.Payload)
}

// Publish message
rdb.Publish(ctx, "notifications", "hello")
```

#### Pattern Subscribe

```go
// Subscribe to pattern
pubsub, err := rdb.PSubscribe(ctx, "events:*")
if err != nil {
    return err
}
defer pubsub.Close()

for msg := range pubsub.Channel() {
    fmt.Printf("Channel: %s, Pattern: %s, Payload: %s\n",
        msg.Channel, msg.Pattern, msg.Payload)
}
```

#### Pipeline

```go
// Use Unwrap() for pipeline operations
pipe := rdb.Unwrap().Pipeline()
incr := pipe.Incr(ctx, "counter")
pipe.Expire(ctx, "counter", time.Hour)
_, err := pipe.Exec(ctx)
if err != nil {
    return err
}
count, _ := incr.Result()
```

#### Transaction

```go
// Use Unwrap() for transaction operations
err := rdb.Unwrap().Watch(ctx, func(tx *redis.Tx) error {
    val, err := tx.Get(ctx, "key").Int()
    if err != nil && err != redis.Nil {
        return err
    }

    _, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
        pipe.Set(ctx, "key", val+1, 0)
        return nil
    })
    return err
}, "key")
```

#### Pool Statistics

```go
stats := rdb.PoolStats()
log.Info("pool stats",
    zap.Uint32("hits", stats.Hits),
    zap.Uint32("misses", stats.Misses),
    zap.Uint32("timeouts", stats.Timeouts),
    zap.Uint32("total_conns", stats.TotalConns),
    zap.Uint32("idle_conns", stats.IdleConns),
)
```

### Error Handling

```go
// Check for non-existent key
val, err := rdb.Get(ctx, "key").Result()
if err == cache.Nil {
    // Key does not exist
} else if err != nil {
    // Other error
}

// Error constructors
ErrInvalidRedisConfig(msg string) error  // Invalid configuration
ErrRedisConnection(err error) error       // Connection failed
ErrRedisOperation(op string, err error) error  // Operation failed
```

## License

This project is licensed under the MIT License - see the [LICENSE](../LICENSE) file for details.
