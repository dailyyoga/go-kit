# nexgo

一个用于常见基础设施模式的 Go 工具包集合。

[English](./README.md) | 简体中文

## 包列表

### [logger](./logger) - 统一日志接口

基于 zap 的统一日志接口，支持可配置的日志级别、编码格式和输出路径。

- 标准 Logger 接口，兼容 *zap.Logger
- 可配置日志级别（debug、info、warn、error 等）
- 多种编码格式（JSON、Console）
- 灵活的输出配置
- 在所有包中统一使用

### [db](./db) - MySQL 数据库客户端

基于 GORM 的 MySQL 数据库客户端，具备连接池管理和结构化日志功能。

- 基于 GORM v2，提供强大的 ORM 能力
- 可配置的连接池管理
- 结构化日志记录，支持慢查询检测
- 自定义日志集成，使用 zap 后端
- 默认启用预编译语句
- 支持上下文感知操作，实现取消和超时控制

[查看文档 →](./db/README.md)

### [ch](./ch) - ClickHouse 客户端

统一的 ClickHouse 客户端，同时支持查询操作和异步批量写入。

- 查询和写入共享连接
- 异步批量写入，双重刷新触发机制（时间和大小）
- 自动类型转换
- 表结构发现和缓存

[查看文档 →](./ch/README.md)

### [kafka](./kafka) - Kafka 消费者/生产者

基于 confluent-kafka-go 构建的轻量级 Kafka 消费者和生产者封装。

- 简洁直观的 API
- 消费者：支持多实例并行处理和自动重试
- 生产者：异步投递，可配置批处理和压缩
- 灵活的偏移量管理和可靠性配置

[查看文档 →](./kafka/README.md)

### [cron](./cron) - 定时任务管理器

高性能定时任务管理器，支持链式任务执行和中间件。

- 链式顺序执行
- 中间件支持（恢复、日志记录）
- 线程安全的任务间数据共享
- 自动 panic 恢复

[查看文档 →](./cron/README.md)

### [routine](./routine) - 安全 Goroutine 执行

安全的 goroutine 执行，自动 panic 恢复以防止应用程序崩溃。

- 自动 panic 恢复，记录堆栈跟踪
- Runner 接口用于跟踪和等待 goroutine
- 独立函数用于即发即弃场景
- 命名 goroutine 以便更好地调试和记录日志

### [cache](./cache) - 可同步缓存 & Redis 客户端

支持周期性同步的缓存和 Redis 客户端封装。

**SyncableCache 可同步缓存：**
- 基于 Go 泛型，支持任意数据类型的通用缓存
- 从可配置数据源周期性后台同步
- 自动重试，指数退避处理瞬态故障
- 线程安全，在同步期间支持并发读取
- 上下文感知，每次同步可配置超时
- **引用类型安全性**：对于切片、映射、指针类型，`Get()` 返回引用 - 请将其视为只读以避免数据竞争

**Redis：**
- 基于 go-redis v9 的轻量封装，通过 `redis.Cmdable` 提供 200+ 命令
- 可配置的连接池管理
- 线程安全操作
- 支持 Pub/Sub 订阅确认
- 连接池状态监控

[查看文档 →](./cache/README.md)

## 安装

```bash
go get github.com/dailyyoga/nexgo
```

## 快速开始

### Logger 日志

**默认配置：**
```go
import "github.com/dailyyoga/nexgo/logger"

// 使用默认配置（info 级别、JSON 编码、stdout 输出）
log, _ := logger.New(nil)
defer log.Sync()

log.Info("application started")
```

**自定义配置：**
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

### Database 数据库

**基本用法：**
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

// 获取 GORM 实例
gormDB, _ := database.DB()

// CRUD 操作
type User struct {
    ID   int64  `gorm:"primaryKey"`
    Name string `gorm:"size:100"`
    Age  int
}

gormDB.AutoMigrate(&User{})
gormDB.Create(&User{Name: "Alice", Age: 25})

// 带上下文的查询
ctx := context.Background()
var user User
gormDB.WithContext(ctx).First(&user, "name = ?", "Alice")
```

**健康检查：**
```go
// 带超时的数据库 Ping
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

**消费者：**
```go
import (
    "github.com/dailyyoga/nexgo/kafka"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
consumer, _ := kafka.NewConsumer(log, config)
defer consumer.Close()

consumer.Start(ctx, func(ctx context.Context, msg *kafka.Message) error {
    // 处理消息
    return nil
})
```

**生产者：**
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

### Cron 定时任务

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

### Routine 协程

**使用 Runner（跟踪和等待）：**
```go
import (
    "github.com/dailyyoga/nexgo/routine"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)
runner := routine.New(log)

runner.GoNamed("process-data", func() {
    // 可能会 panic 的工作 - 将被安全恢复
})

runner.GoWithContext(ctx, func(ctx context.Context) {
    // 带上下文的工作
})

runner.Wait() // 等待所有 goroutine 完成
```

**独立函数（即发即弃）：**
```go
import (
    "github.com/dailyyoga/nexgo/routine"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)

// 简单的一次性 goroutine
routine.Go(log, func() {
    // 工作 - panic 将被恢复并记录
})

// 命名以便更好地记录日志
routine.GoNamed(log, "background-task", func() {
    // 工作
})
```

### Cache 缓存

```go
import (
    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
)

log, _ := logger.New(nil)

// 定义同步函数
syncFunc := func(ctx context.Context) ([]User, error) {
    return fetchUsersFromDB(ctx)
}

// 创建并启动缓存
cfg := &cache.SyncableCacheConfig{
    Name:         "user-cache",        // 必填字段
    SyncInterval: 5 * time.Minute,
    SyncTimeout:  30 * time.Second,
    MaxRetries:   3,
}
c, _ := cache.NewSyncableCache(log, cfg, syncFunc)
c.Start() // 阻塞直到初始同步成功
defer c.Stop()

// 获取缓存数据（线程安全，只读）
users := c.Get()

// 重要提示：对于引用类型（切片、映射、指针），Get() 返回
// 缓存数据的引用。请将其视为只读：
// ✅ 正确：for _, u := range users { fmt.Println(u.Name) }
// ❌ 危险：users[0].Name = "modified"  // 数据竞争！
// 如果需要修改，请先创建副本
```

### Redis

```go
import (
    "github.com/dailyyoga/nexgo/cache"
    "github.com/dailyyoga/nexgo/logger"
    "github.com/redis/go-redis/v9"
)

log, _ := logger.New(nil)

// 创建 Redis 客户端
cfg := &cache.RedisConfig{
    Addr:     "localhost:6379",
    PoolSize: 10,
}
rdb, _ := cache.NewRedis(log, cfg)
defer rdb.Close()

ctx := context.Background()

// 字符串操作
rdb.Set(ctx, "key", "value", time.Hour)
val, _ := rdb.Get(ctx, "key").Result()

// 检查键是否存在
if err == cache.Nil {
    // 键不存在
}

// 哈希操作
rdb.HSet(ctx, "user:1", "name", "Alice", "age", "25")

// 发布/订阅
pubsub, _ := rdb.Subscribe(ctx, "channel")
defer pubsub.Close()

// 管道操作（通过 Unwrap）
pipe := rdb.Unwrap().Pipeline()
pipe.Incr(ctx, "counter")
pipe.Expire(ctx, "counter", time.Hour)
pipe.Exec(ctx)
```

## 依赖

- Go 1.24.6+
- [ClickHouse/clickhouse-go/v2](https://github.com/ClickHouse/clickhouse-go)
- [confluentinc/confluent-kafka-go/v2](https://github.com/confluentinc/confluent-kafka-go)
- [redis/go-redis/v9](https://github.com/redis/go-redis)
- [robfig/cron/v3](https://github.com/robfig/cron)
- [uber-go/zap](https://github.com/uber-go/zap)

## 开发

```bash
# 运行测试
go test ./...

# 格式化代码
go fmt ./...

# 检查代码
go vet ./...
```

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。
