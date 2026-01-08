// Package cache provides cache implementations with various strategies.
//
// The cache package follows go-kit conventions:
// - Interface-driven design for testability
// - Uses logger.Logger interface for unified logging
// - Uses routine package for safe goroutine execution
// - Configuration with validation and defaults
// - Structured error handling
//
// Available cache implementations:
// - SyncableCache: A cache that periodically syncs data from a source
// - Redis: A Redis client wrapper with connection pool management
package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// SyncFunc is a function that performs the actual sync operation
// It should return the new cache data or an error
// The context should be respected for cancellation and timeout
type SyncFunc[T any] func(ctx context.Context) (T, error)

// SyncableCache is a cache that periodically syncs data from a source
// It provides thread-safe read access to cached data while performing
// periodic background synchronization with automatic retry logic
type SyncableCache[T any] interface {
	// Start begins the periodic sync process
	// It performs an initial sync before starting the background goroutine
	// Returns error if initial sync fails
	Start() error

	// Stop gracefully stops the periodic sync process
	// It can be called multiple times safely
	Stop()

	// Get returns the current cached value
	// It is safe to call concurrently with sync operations
	//
	// IMPORTANT: For reference types (slice, map, pointer, chan), Get() returns
	// a reference to the cached data, not a deep copy. Callers MUST treat the
	// returned value as read-only. Modifying the returned value will cause
	// data races and undefined behavior when accessed by other goroutines.
	//
	// For value types (int, string, struct without pointers), this is not a concern
	// as Go automatically copies the value.
	//
	// Safe usage examples:
	//   // Value type - automatically copied
	//   count := cache.Get()  // T = int
	//
	//   // Reference type - read-only access
	//   users := cache.Get()  // T = []User
	//   for _, user := range users {
	//       fmt.Println(user.Name)  // OK - read-only
	//   }
	//
	// Unsafe usage examples:
	//   users := cache.Get()  // T = []User
	//   users[0].Name = "modified"  // DANGER - data race!
	//
	// If you need to modify the data, create a deep copy first:
	//   users := cache.Get()
	//   usersCopy := make([]User, len(users))
	//   copy(usersCopy, users)
	//   usersCopy[0].Name = "modified"  // OK - modifying copy
	Get() T

	// Sync manually triggers a sync operation
	// The context can be used to set timeout or cancel the operation
	// Returns error if sync fails after all retry attempts
	Sync(ctx context.Context) error
}

// Redis is the interface for the Redis client
// It embeds redis.Cmdable to provide access to all Redis commands
type Redis interface {
	redis.Cmdable

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

// Nil is redis.Nil - returned when key does not exist
var Nil = redis.Nil
