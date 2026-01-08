package cache

import (
	"context"

	"github.com/dailyyoga/nexgo/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// defaultRedis embeds *redis.Client to implement redis.Cmdable automatically
type defaultRedis struct {
	*redis.Client
	log logger.Logger
}

// NewRedis creates a new Redis client with the given configuration
func NewRedis(log logger.Logger, cfg *RedisConfig) (Redis, error) {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	} else {
		cfg = cfg.MergeDefaults()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client := redis.NewClient(cfg.Options())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, ErrRedisConnection(err)
	}

	log.Info("redis client connected", zap.String("addr", cfg.Addr), zap.Int("db", cfg.DB))
	return &defaultRedis{Client: client, log: log}, nil
}

// Close closes the client connection
func (r *defaultRedis) Close() error {
	if err := r.Client.Close(); err != nil {
		return ErrRedisOperation("CLOSE", err)
	}
	r.log.Info("redis client closed")
	return nil
}

// Unwrap returns the underlying redis.Client
func (r *defaultRedis) Unwrap() *redis.Client {
	return r.Client
}

// PoolStats returns connection pool statistics
func (r *defaultRedis) PoolStats() *redis.PoolStats {
	return r.Client.PoolStats()
}

// Subscribe subscribes to channels and waits for confirmation
func (r *defaultRedis) Subscribe(ctx context.Context, channels ...string) (*redis.PubSub, error) {
	pubsub := r.Client.Subscribe(ctx, channels...)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, ErrRedisOperation("SUBSCRIBE", err)
	}
	r.log.Debug("subscribed", zap.Strings("channels", channels))
	return pubsub, nil
}

// PSubscribe subscribes to channel patterns and waits for confirmation
func (r *defaultRedis) PSubscribe(ctx context.Context, patterns ...string) (*redis.PubSub, error) {
	pubsub := r.Client.PSubscribe(ctx, patterns...)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, ErrRedisOperation("PSUBSCRIBE", err)
	}
	r.log.Debug("pattern subscribed", zap.Strings("patterns", patterns))
	return pubsub, nil
}
