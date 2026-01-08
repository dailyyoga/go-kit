package cache

import (
	"fmt"
	"time"
)

// Predefined errors
var (
	// ErrCacheClosed is returned when operations are attempted on a closed cache
	ErrCacheClosed = fmt.Errorf("cache: cache is closed")
	// ErrInvalidConfig is returned when the configuration is invalid
	ErrInvalidConfig = fmt.Errorf("cache: invalid config")
)

// Error constructors

// ErrSync wraps a sync operation error
func ErrSync(err error) error {
	return fmt.Errorf("cache: sync failed: %w", err)
}

// ErrInvalidName returns an error for invalid name
func ErrInvalidName(name string) error {
	return fmt.Errorf("cache: invalid name: %s (must be non-empty)", name)
}

// ErrInvalidSyncInterval returns an error for invalid sync interval
func ErrInvalidSyncInterval(interval time.Duration) error {
	return fmt.Errorf("cache: invalid sync interval: %v (must be > 0)", interval)
}

// ErrInvalidSyncTimeout returns an error for invalid sync timeout
func ErrInvalidSyncTimeout(timeout time.Duration) error {
	return fmt.Errorf("cache: invalid sync timeout: %v (must be > 0)", timeout)
}

// ErrInvalidMaxRetries returns an error for invalid max retries
func ErrInvalidMaxRetries(retries int) error {
	return fmt.Errorf("cache: invalid max retries: %d (must be >= 1)", retries)
}

// ErrInvalidRedisConfig returns an error for invalid Redis configuration
func ErrInvalidRedisConfig(msg string) error {
	return fmt.Errorf("cache: invalid redis config: %s", msg)
}

// ErrRedisConnection wraps a Redis connection error
func ErrRedisConnection(err error) error {
	return fmt.Errorf("cache: redis connection failed: %w", err)
}

// ErrRedisOperation wraps a Redis operation error
func ErrRedisOperation(op string, err error) error {
	return fmt.Errorf("cache: redis %s failed: %w", op, err)
}
