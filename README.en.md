# Rate Limit Testing Framework

A comprehensive Go framework for implementing and testing rate limiting algorithms. This project provides sample implementations of various rate limiting strategies along with client-server tools for validation and benchmarking.

## Overview

This project consists of:

- **Test Client**: Sends requests at configurable rates
- **Test Server**: Acts as an echo server
- **Algorithm Examples**: 4 different rate limiting implementations

## Quick Start

### Basic Usage

1. Start the server:
```bash
go run server/main.go -protocol tcp -port 8080
```

2. In another terminal, start the client:
```bash
go run main.go -server localhost:8080 -rate 100 -duration 30s
```

### Examples

```bash
# Start TCP server with verbose logging
go run server/main.go -protocol tcp -port 8080 -verbose

# High-rate test (1000 req/s, 10 connections)
go run main.go -rate 1000 -connections 10 -duration 60s

# UDP protocol test
go run server/main.go -protocol udp -port 9090
go run main.go -protocol udp -server localhost:9090 -rate 500
```

## Architecture

### Directory Structure

```
.
├── main.go                    # Test client
├── server/
│   └── main.go               # Test server
└── sample/
    ├── token_bucket/         # Token bucket implementation
    ├── fixed_window/         # Fixed window implementation
    ├── sliding_window/       # Sliding window implementation
    └── concurrent/           # Concurrent-safe implementation
```

## Component Details

### Test Client (main.go)

A high-performance rate limiting test client.

**Features:**
- TCP/UDP protocol support
- Multiple concurrent connections (TCP only)
- Customizable message size
- Real-time statistics display

**Command-line Options:**
```bash
-server string     # Server address (default "localhost:8080")
-protocol string   # Protocol: tcp or udp (default "tcp")
-rate int         # Messages per second (default 100)
-duration duration # Test duration (default 10s)
-connections int   # Concurrent connections, TCP only (default 1)
-size int         # Message size in bytes (default 64)
```

**Output Example:**
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

### Test Server (server/main.go)

A simple server that echoes received messages.

**Features:**
- TCP/UDP protocol support
- Multiple simultaneous client connections
- Statistics display every 5 seconds
- Graceful shutdown (Ctrl+C)

**Command-line Options:**
```bash
-protocol string  # Protocol: tcp or udp (default "tcp")
-port int        # Listening port (default 8080)
-verbose         # Enable verbose logging
```

**Statistics Display Example:**
```
[15:30:45] Received: 5023, Processed: 5023, Errors: 0, Rate: 1004.60 msg/s
[15:30:50] Received: 10089, Processed: 10089, Errors: 0, Rate: 1013.20 msg/s
```

## Rate Limiting Algorithms

### 1. Token Bucket

**Features:**
- Allows burst processing
- Maintains average rate while permitting short-term spikes
- Memory efficient

**Usage Example:**
```go
limiter := NewTokenBucket(100, 10) // Capacity 100, refill 10/second

if limiter.Allow() {
    // Process request
}
```

**Use Cases:**
- API rate limiting
- Network bandwidth control
- When burst traffic is acceptable

### 2. Fixed Window

**Features:**
- Simple implementation
- Minimal memory usage
- Boundary issues at window edges

**Usage Example:**
```go
limiter := NewFixedWindowLimiter(1000, time.Minute) // 1000 requests per minute

if limiter.Allow() {
    // Process request
}
```

**Use Cases:**
- When simple rate limiting is needed
- Performance is prioritized over accuracy

### 3. Sliding Window

**Features:**
- More accurate rate limiting
- Solves fixed window boundary issues
- Slightly higher memory usage

**Usage Example:**
```go
limiter := NewSlidingWindowLimiter(100, time.Minute) // 100 requests per minute

if limiter.Allow() {
    // Process request
}
```

**Use Cases:**
- When accurate rate limiting is required
- Fairness is important

### 4. Concurrent Implementation

**Features:**
- High performance with atomic operations
- Distributed environment support
- HTTP middleware example

**HTTP Middleware Usage:**
```go
// Per-user rate limiting
userLimiters := &UserRateLimiters{
    limiters: make(map[string]*ConcurrentTokenBucket),
    limit:    100,  // 100 requests/minute per user
}

mux := http.NewServeMux()
mux.HandleFunc("/api/", handler)

// Apply middleware
http.ListenAndServe(":8080", RateLimitMiddleware(userLimiters)(mux))
```

## Performance Benchmarks

Algorithm comparison (reference values):

| Algorithm | Throughput | Memory Usage | Features |
|-----------|------------|--------------|----------|
| Token Bucket | High | Low | Burst support |
| Fixed Window | Highest | Lowest | Simple |
| Sliding Window | Medium | Medium | Accurate |
| Concurrent | High | Low | Concurrency optimized |

## Practical Use Cases

### 1. API Server Rate Limit Testing

```bash
# Test API server rate limiting (1000 req/s, 5 minutes)
go run main.go -server api.example.com:443 -rate 1000 -duration 5m -connections 50
```

### 2. Load Test Scenarios

```bash
# Gradual load increase test
for rate in 100 500 1000 2000 5000; do
    echo "Testing rate: $rate req/s"
    go run main.go -rate $rate -duration 1m
    sleep 10
done
```

### 3. Network Bandwidth Measurement

```bash
# Bandwidth measurement with large message size
go run main.go -size 1024 -rate 1000 -protocol udp
```

## Troubleshooting

### Common Issues

1. **"Too many open files" error**
   ```bash
   # Increase file descriptor limit
   ulimit -n 65536
   ```

2. **Connection errors at high rates**
   - Adjust `-connections` parameter
   - Check server-side buffer sizes

3. **UDP packet loss**
   - Reduce rate or message size
   - Adjust kernel UDP buffer size

## Development

### Building

```bash
# Build client
go build -o rate-limit-client main.go

# Build server
go build -o rate-limit-server server/main.go
```

### Testing

```bash
# Run unit tests
go test ./...

# Run benchmarks
go test -bench=. ./sample/...
```

### Adding New Algorithms

1. Create new package in `sample/` directory
2. Implement `RateLimiter` interface:
   ```go
   type RateLimiter interface {
       Allow() bool
       AllowN(n int) bool
   }
   ```
3. Add tests and benchmarks

## License

MIT License

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## References

- [Rate Limiting Algorithms](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Token Bucket Algorithm](https://en.wikipedia.org/wiki/Token_bucket)
- [Sliding Window Counter](https://blog.logrocket.com/rate-limiting-go-application/)