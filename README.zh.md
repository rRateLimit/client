# 速率限制测试框架

一个全面的 Go 语言框架，用于实现和测试速率限制算法。该项目提供了各种速率限制策略的示例实现，以及用于验证和基准测试的客户端-服务器工具。

## 概述

本项目包含：

- **测试客户端**：以可配置的速率发送请求
- **测试服务器**：作为回声服务器运行
- **算法示例**：4 种不同的速率限制实现

## 快速开始

### 基本用法

1. 启动服务器：
```bash
go run server/main.go -protocol tcp -port 8080
```

2. 在另一个终端启动客户端：
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### 示例

```bash
# 启动带详细日志的 TCP 服务器
go run server/main.go -protocol tcp -port 8080 -verbose

# 高速率测试（1000 请求/秒，10 个连接）
go run main.go -rate 1000 -connections 10 -duration 60s

# UDP 协议测试
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## 架构

### 目录结构

```
.
├── main.go                    # 测试客户端
├── server/
│   └── main.go               # 测试服务器
└── sample/
    ├── token_bucket/         # 令牌桶实现
    ├── fixed_window/         # 固定窗口实现
    ├── sliding_window/       # 滑动窗口实现
    └── concurrent/           # 线程安全实现
```

## 组件详情

### 测试客户端（main.go）

一个高性能的速率限制测试客户端。

**功能：**
- TCP/UDP 协议支持
- 多并发连接（仅 TCP）
- 可自定义消息大小
- 实时统计显示

**命令行选项：**
```bash
-server string     # 服务器地址（默认 "localhost:8080"）
-protocol string   # 协议：tcp 或 udp（默认 "tcp"）
-rate int         # 每秒消息数（默认 100）
-duration duration # 测试持续时间（默认 10s）
-connections int   # 并发连接数，仅 TCP（默认 1）
-size int         # 消息大小（字节）（默认 64）
```

**输出示例：**
```
Starting rate limit test client
Protocol: tcp
Server: localhost:8080
Rate: 1000 messages/second
Duration: 30s
Connections: 10
Message size: 64 bytes

--- Test Statistics ---
Duration: 30s
Messages sent: 30000
Messages succeeded: 29850
Messages failed: 150
Success rate: 99.50%
Actual rate: 1000.00 messages/second
```

### 测试服务器（server/main.go）

一个简单的回声服务器。

**功能：**
- TCP/UDP 协议支持
- 多客户端同时连接
- 每 5 秒显示统计信息
- 优雅关闭（Ctrl+C）

**命令行选项：**
```bash
-protocol string  # 协议：tcp 或 udp（默认 "tcp"）
-port int        # 监听端口（默认 8080）
-verbose         # 启用详细日志
```

**统计显示示例：**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## 速率限制算法

### 1. 令牌桶（Token Bucket）

**特点：**
- 允许突发处理
- 保持平均速率同时允许短期峰值
- 内存效率高

**使用示例：**
```go
limiter := NewTokenBucket(100, 10) // 容量 100，每秒补充 10 个

if limiter.Allow() {
    // 处理请求
}
```

**使用场景：**
- API 速率限制
- 网络带宽控制
- 可接受突发流量时

### 2. 固定窗口（Fixed Window）

**特点：**
- 实现简单
- 内存使用最少
- 窗口边界存在问题

**使用示例：**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 每分钟 1000 个请求

if limiter.Allow() {
    // 处理请求
}
```

**使用场景：**
- 需要简单速率限制时
- 性能优先于精度

### 3. 滑动窗口（Sliding Window）

**特点：**
- 更精确的速率限制
- 解决固定窗口边界问题
- 内存使用稍高

**使用示例：**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 每分钟 100 个请求

if limiter.Allow() {
    // 处理请求
}
```

**使用场景：**
- 需要精确速率限制时
- 公平性很重要时

### 4. 并发实现（Concurrent）

**特点：**
- 使用原子操作的高性能
- 分布式环境支持
- HTTP 中间件示例

**HTTP 中间件使用：**
```go
// 用户级速率限制
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 每用户每分钟 100 个请求
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// 应用中间件
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## 性能基准测试

算法比较（参考值）：

| 算法 | 吞吐量 | 内存使用 | 特点 |
|------|--------|----------|------|
| 令牌桶 | 高 | 低 | 支持突发 |
| 固定窗口 | 最高 | 最低 | 简单 |
| 滑动窗口 | 中 | 中 | 精确 |
| 并发 | 高 | 低 | 并发优化 |

## 实际用例

### 1. API 服务器速率限制测试

```bash
# 测试 API 服务器速率限制（1000 请求/秒，5 分钟）
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. 负载测试场景

```bash
# 渐进式负载增加测试
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. 网络带宽测量

```bash
# 使用大消息进行带宽测量
go run main.go -size 1024 -rate 1000 -protocol udp
```

## 故障排除

### 常见问题

1. **"Too many open files" 错误**
   ```bash
   # 增加文件描述符限制
   ulimit -n 65536
   ```

2. **高速率连接错误**
   - 调整 `-connections` 参数
   - 检查服务器端缓冲区大小

3. **UDP 数据包丢失**
   - 降低速率或消息大小
   - 调整内核 UDP 缓冲区大小

## 开发

### 构建

```bash
# 构建客户端
go build -o rate-limit-client main.go

# 构建服务器
go build -o rate-limit-server server/main.go
```

### 测试

```bash
# 运行单元测试
go test ./...

# 运行基准测试
go test -bench=. ./sample/...
```

### 添加新算法

1. 在 `sample/` 目录创建新包
2. 实现 `RateLimiter` 接口：
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. 添加测试和基准测试

## 许可证

MIT 许可证

## 贡献

欢迎提交 Pull Request。对于重大更改，请先开启 issue 讨论您想要更改的内容。

## 参考资料

- [速率限制算法](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [令牌桶算法](https://zh.wikipedia.org/wiki/令牌桶)
- [滑动窗口计数器](https://blog.logrocket.com/rate-limiting-go-application/)