package cache

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// SyncableCacheConfig holds configuration for SyncableCache
type SyncableCacheConfig struct {
	// Name is used for logging purposes to identify the cache (required)
	Name string `mapstructure:"name"`
	// SyncInterval is the interval between periodic sync operations
	// default: 5 * time.Minute
	SyncInterval time.Duration `mapstructure:"sync_interval"`
	// SyncTimeout is the timeout for each sync operation
	// default: 30 * time.Second
	SyncTimeout time.Duration `mapstructure:"sync_timeout"`
	// MaxRetries is the maximum number of retry attempts for failed sync operations
	// default: 3
	MaxRetries int `mapstructure:"max_retries"`
}

// DefaultSyncableCacheConfig returns the default configuration for SyncableCache
// It is used to initialize the cache with default configuration when no configuration is provided
// Note: Name field has no default value and must be explicitly set by the user
func DefaultSyncableCacheConfig() *SyncableCacheConfig {
	return &SyncableCacheConfig{
		// Name is required and must be explicitly set
		SyncInterval: 5 * time.Minute,
		SyncTimeout:  30 * time.Second,
		MaxRetries:   3,
	}
}

// Validate validates the configuration
// It checks that all required fields are set and have valid values
func (c *SyncableCacheConfig) Validate() error {
	if c.Name == "" {
		return ErrInvalidName(c.Name)
	}
	if c.SyncInterval <= 0 {
		return ErrInvalidSyncInterval(c.SyncInterval)
	}
	if c.SyncTimeout <= 0 {
		return ErrInvalidSyncTimeout(c.SyncTimeout)
	}
	if c.MaxRetries < 1 {
		return ErrInvalidMaxRetries(c.MaxRetries)
	}
	return nil
}

// RedisConfig holds configuration for Redis client
type RedisConfig struct {
	Addr            string        `mapstructure:"addr"`               // Redis address (default: localhost:6379)
	Password        string        `mapstructure:"password"`           // Password for auth (default: "")
	DB              int           `mapstructure:"db"`                 // Database number (default: 0)
	PoolSize        int           `mapstructure:"pool_size"`          // Max connections (default: 10)
	MinIdleConns    int           `mapstructure:"min_idle_conns"`     // Min idle connections (default: 5)
	MaxRetries      int           `mapstructure:"max_retries"`        // Max retries (default: 3)
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`       // Dial timeout (default: 5s)
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`       // Read timeout (default: 3s)
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`      // Write timeout (default: 3s)
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"` // Max idle time (default: 5m)
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`  // Max lifetime (default: 0)
}

// DefaultRedisConfig returns the default configuration
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:            "localhost:6379",
		PoolSize:        10,
		MinIdleConns:    5,
		MaxRetries:      3,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// Options converts config to redis.Options
func (c *RedisConfig) Options() *redis.Options {
	return &redis.Options{
		Addr:            c.Addr,
		Password:        c.Password,
		DB:              c.DB,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		MaxRetries:      c.MaxRetries,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		ConnMaxIdleTime: c.ConnMaxIdleTime,
		ConnMaxLifetime: c.ConnMaxLifetime,
	}
}

// Validate validates the configuration
func (c *RedisConfig) Validate() error {
	if c.Addr == "" {
		return ErrInvalidRedisConfig("addr is required")
	}
	if c.DB < 0 || c.PoolSize < 0 || c.MinIdleConns < 0 || c.MaxRetries < 0 {
		return ErrInvalidRedisConfig("numeric fields must be >= 0")
	}
	if c.DialTimeout < 0 || c.ReadTimeout < 0 || c.WriteTimeout < 0 {
		return ErrInvalidRedisConfig("timeout fields must be >= 0")
	}
	return nil
}

// MergeDefaults merges defaults for zero-value fields
func (c *RedisConfig) MergeDefaults() *RedisConfig {
	d := DefaultRedisConfig()
	if c.Addr == "" {
		c.Addr = d.Addr
	}
	if c.PoolSize == 0 {
		c.PoolSize = d.PoolSize
	}
	if c.MinIdleConns == 0 {
		c.MinIdleConns = d.MinIdleConns
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = d.MaxRetries
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = d.DialTimeout
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = d.ReadTimeout
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = d.WriteTimeout
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = d.ConnMaxIdleTime
	}
	return c
}
