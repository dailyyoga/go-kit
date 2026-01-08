package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dailyyoga/nexgo/logger"
	"github.com/redis/go-redis/v9"
)

func testLogger(t *testing.T) logger.Logger {
	t.Helper()
	log, _ := logger.New(&logger.Config{Level: "debug", Encoding: "console"})
	return log
}

func setupTestRedis(t *testing.T) (Redis, *miniredis.Miniredis) {
	t.Helper()
	mr, _ := miniredis.Run()
	rdb, err := NewRedis(testLogger(t), &RedisConfig{Addr: mr.Addr(), DialTimeout: time.Second})
	if err != nil {
		mr.Close()
		t.Fatalf("failed to create redis: %v", err)
	}
	return rdb, mr
}

// ============ Config Tests ============

func TestRedisConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *RedisConfig
		wantErr bool
	}{
		{"valid", &RedisConfig{Addr: "localhost:6379"}, false},
		{"empty addr", &RedisConfig{}, true},
		{"negative db", &RedisConfig{Addr: "localhost:6379", DB: -1}, true},
		{"negative pool", &RedisConfig{Addr: "localhost:6379", PoolSize: -1}, true},
		{"negative timeout", &RedisConfig{Addr: "localhost:6379", DialTimeout: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedisConfig_MergeDefaults(t *testing.T) {
	cfg := (&RedisConfig{Addr: "custom:6379"}).MergeDefaults()
	if cfg.Addr != "custom:6379" || cfg.PoolSize != 10 || cfg.DialTimeout != 5*time.Second {
		t.Error("MergeDefaults failed")
	}
}

func TestRedisConfig_Options(t *testing.T) {
	cfg := &RedisConfig{Addr: "localhost:6379", DB: 1, PoolSize: 20}
	opts := cfg.Options()
	if opts.Addr != "localhost:6379" || opts.DB != 1 || opts.PoolSize != 20 {
		t.Error("Options conversion failed")
	}
}

// ============ String Operations ============

func TestRedis_GetSet(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.Set(ctx, "key1", "value1", 0)
	if val, _ := rdb.Get(ctx, "key1").Result(); val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
	if _, err := rdb.Get(ctx, "nonexistent").Result(); err != redis.Nil {
		t.Errorf("expected redis.Nil, got %v", err)
	}
}

func TestRedis_SetNX(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	ok, _ := rdb.SetNX(ctx, "lock", "1", time.Minute).Result()
	if !ok {
		t.Error("first SetNX should succeed")
	}
	ok, _ = rdb.SetNX(ctx, "lock", "2", time.Minute).Result()
	if ok {
		t.Error("second SetNX should fail")
	}
}

func TestRedis_IncrDecr(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	if v, _ := rdb.Incr(ctx, "c").Result(); v != 1 {
		t.Errorf("Incr: expected 1, got %d", v)
	}
	if v, _ := rdb.IncrBy(ctx, "c", 5).Result(); v != 6 {
		t.Errorf("IncrBy: expected 6, got %d", v)
	}
	if v, _ := rdb.Decr(ctx, "c").Result(); v != 5 {
		t.Errorf("Decr: expected 5, got %d", v)
	}
	if v, _ := rdb.DecrBy(ctx, "c", 3).Result(); v != 2 {
		t.Errorf("DecrBy: expected 2, got %d", v)
	}
}

// ============ Key Operations ============

func TestRedis_Del(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.Set(ctx, "k1", "v1", 0)
	rdb.Set(ctx, "k2", "v2", 0)
	if n, _ := rdb.Del(ctx, "k1", "k2", "k3").Result(); n != 2 {
		t.Errorf("expected 2 deleted, got %d", n)
	}
}

func TestRedis_Exists(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.Set(ctx, "k1", "v1", 0)
	if n, _ := rdb.Exists(ctx, "k1", "k2").Result(); n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestRedis_ExpireTTL(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.Set(ctx, "k1", "v1", 0)
	rdb.Expire(ctx, "k1", time.Minute)
	if ttl, _ := rdb.TTL(ctx, "k1").Result(); ttl <= 0 {
		t.Errorf("unexpected TTL: %v", ttl)
	}
}

// ============ Hash Operations ============

func TestRedis_Hash(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.HSet(ctx, "h", "f1", "v1", "f2", "v2")
	if v, _ := rdb.HGet(ctx, "h", "f1").Result(); v != "v1" {
		t.Errorf("expected v1, got %s", v)
	}
	if m, _ := rdb.HGetAll(ctx, "h").Result(); len(m) != 2 {
		t.Errorf("expected 2 fields, got %d", len(m))
	}
	if ok, _ := rdb.HExists(ctx, "h", "f1").Result(); !ok {
		t.Error("field should exist")
	}
	rdb.HDel(ctx, "h", "f1")
	if ok, _ := rdb.HExists(ctx, "h", "f1").Result(); ok {
		t.Error("field should not exist")
	}
}

// ============ List Operations ============

func TestRedis_List(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.LPush(ctx, "l", "a", "b", "c")
	rdb.RPush(ctx, "l", "d")
	if n, _ := rdb.LLen(ctx, "l").Result(); n != 4 {
		t.Errorf("expected 4, got %d", n)
	}
	if v, _ := rdb.LPop(ctx, "l").Result(); v != "c" {
		t.Errorf("expected c, got %s", v)
	}
	if v, _ := rdb.RPop(ctx, "l").Result(); v != "d" {
		t.Errorf("expected d, got %s", v)
	}
}

// ============ Set Operations ============

func TestRedis_Set(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.SAdd(ctx, "s", "a", "b", "c")
	if n, _ := rdb.SCard(ctx, "s").Result(); n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
	if ok, _ := rdb.SIsMember(ctx, "s", "a").Result(); !ok {
		t.Error("a should be member")
	}
	rdb.SRem(ctx, "s", "a")
	if ok, _ := rdb.SIsMember(ctx, "s", "a").Result(); ok {
		t.Error("a should not be member")
	}
}

// ============ Sorted Set Operations ============

func TestRedis_ZSet(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.ZAdd(ctx, "z", redis.Z{Score: 1, Member: "a"}, redis.Z{Score: 2, Member: "b"})
	if n, _ := rdb.ZCard(ctx, "z").Result(); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if s, _ := rdb.ZScore(ctx, "z", "b").Result(); s != 2 {
		t.Errorf("expected 2, got %f", s)
	}
}

// ============ Script Operations ============

func TestRedis_Eval(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	if r, _ := rdb.Eval(ctx, `return ARGV[1]`, nil, "hello").Result(); r != "hello" {
		t.Errorf("expected hello, got %v", r)
	}
}

func TestRedis_ScriptLoadEvalSha(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	sha, _ := rdb.ScriptLoad(ctx, `return ARGV[1]`).Result()
	if r, _ := rdb.EvalSha(ctx, sha, nil, "world").Result(); r != "world" {
		t.Errorf("expected world, got %v", r)
	}
}

// ============ Pub/Sub ============

func TestRedis_PubSub(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	pubsub, err := rdb.Subscribe(ctx, "ch")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer pubsub.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		rdb.Publish(ctx, "ch", "msg")
	}()

	select {
	case msg := <-pubsub.Channel():
		if msg.Payload != "msg" {
			t.Errorf("expected msg, got %s", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

// ============ Connection ============

func TestRedis_Ping(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestRedis_Client(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	if rdb.Unwrap() == nil {
		t.Error("Unwrap() returned nil")
	}
}

func TestNewRedis_ConnectionError(t *testing.T) {
	_, err := NewRedis(testLogger(t), &RedisConfig{Addr: "invalid:9999", DialTimeout: 50 * time.Millisecond})
	if err == nil {
		t.Error("expected error")
	}
}

// ============ Additional Commands ============

func TestRedis_MGetMSet(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	rdb.MSet(ctx, "k1", "v1", "k2", "v2")
	vals, _ := rdb.MGet(ctx, "k1", "k2", "k3").Result()
	if vals[0] != "v1" || vals[1] != "v2" || vals[2] != nil {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestRedis_Pipeline(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	pipe := rdb.Unwrap().Pipeline()
	incr := pipe.Incr(ctx, "counter")
	pipe.Exec(ctx)
	if v, _ := incr.Result(); v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

// ============ Error Path Tests ============

func TestRedisConfig_Validate_AllErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  *RedisConfig
	}{
		{"empty addr", &RedisConfig{}},
		{"negative db", &RedisConfig{Addr: "localhost:6379", DB: -1}},
		{"negative pool size", &RedisConfig{Addr: "localhost:6379", PoolSize: -1}},
		{"negative min idle conns", &RedisConfig{Addr: "localhost:6379", MinIdleConns: -1}},
		{"negative max retries", &RedisConfig{Addr: "localhost:6379", MaxRetries: -1}},
		{"negative dial timeout", &RedisConfig{Addr: "localhost:6379", DialTimeout: -1}},
		{"negative read timeout", &RedisConfig{Addr: "localhost:6379", ReadTimeout: -1}},
		{"negative write timeout", &RedisConfig{Addr: "localhost:6379", WriteTimeout: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestNewRedis_NilConfig(t *testing.T) {
	// NewRedis with nil config should use defaults - which will fail connection
	_, err := NewRedis(testLogger(t), nil)
	if err == nil {
		t.Error("expected connection error with default localhost")
	}
}

func TestNewRedis_InvalidConfig(t *testing.T) {
	_, err := NewRedis(testLogger(t), &RedisConfig{Addr: "", PoolSize: -1})
	if err == nil {
		t.Error("expected validation error")
	}
}

// ============ PSubscribe Test ============

func TestRedis_PSubscribe(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()
	ctx := context.Background()

	pubsub, err := rdb.PSubscribe(ctx, "channel:*")
	if err != nil {
		t.Fatalf("PSubscribe failed: %v", err)
	}
	defer pubsub.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		rdb.Publish(ctx, "channel:test", "pattern-msg")
	}()

	select {
	case msg := <-pubsub.Channel():
		if msg.Payload != "pattern-msg" {
			t.Errorf("expected pattern-msg, got %s", msg.Payload)
		}
		if msg.Pattern != "channel:*" {
			t.Errorf("expected pattern channel:*, got %s", msg.Pattern)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

// ============ PoolStats Test ============

func TestRedis_PoolStats(t *testing.T) {
	rdb, mr := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	stats := rdb.PoolStats()
	if stats == nil {
		t.Error("PoolStats() returned nil")
	}
	// Stats should be valid (TotalConns >= 0)
	if stats.TotalConns < 0 {
		t.Errorf("unexpected TotalConns: %d", stats.TotalConns)
	}
}
