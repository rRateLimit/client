# Rate Limit Test Client

A command-line tool for testing and benchmarking rate-limiting servers by sending messages at a specified rate over network connections.

## Features

- Support for both TCP and UDP protocols
- Configurable message rate and duration
- Multiple concurrent connections (TCP only)
- Real-time statistics tracking
- Customizable message size
- Detailed error reporting

## Installation

```bash
go get github.com/rRateLimit/client
```

Or build from source:

```bash
git clone https://github.com/rRateLimit/client.git
cd client
go build -o rate-limit-client
```

## Usage

```bash
./rate-limit-client [options]
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `-server` | Target server address | `localhost:8080` |
| `-protocol` | Network protocol (`tcp` or `udp`) | `tcp` |
| `-rate` | Number of messages per second | `100` |
| `-duration` | Test duration (e.g., "10s", "1m") | `10s` |
| `-connections` | Number of concurrent TCP connections | `1` |
| `-size` | Size of each message in bytes | `64` |

### Examples

Basic TCP test with default settings:
```bash
./rate-limit-client
```

Test a remote server with higher rate:
```bash
./rate-limit-client -server example.com:9000 -rate 500
```

UDP test with custom duration:
```bash
./rate-limit-client -protocol udp -duration 30s
```

Multiple TCP connections with large messages:
```bash
./rate-limit-client -connections 10 -size 1024 -rate 1000
```

## Output

The client displays real-time statistics during the test:

```
Testing example.com:8080 with protocol: tcp
Rate: 100 msg/s, Duration: 10s, Connections: 1, Message size: 64 bytes

Test completed!
Total messages sent: 1000
Messages acknowledged: 950
Failed messages: 50
Success rate: 95.00%
Actual rate: 100.00 msg/s
```

## How It Works

### TCP Mode
- Creates the specified number of persistent connections to the server
- Each connection runs in a separate goroutine
- Messages are distributed evenly across connections
- Expects acknowledgment for each message sent

### UDP Mode
- Uses a single connection with separate sender and receiver goroutines
- Messages are sent without waiting for acknowledgments
- Receiver goroutine collects responses asynchronously

## Message Format

Messages consist of repeating alphabetic patterns (A-Z) of the specified size. Each message is unique and includes timing information for tracking purposes.

## Requirements

- Go 1.21 or later
- Network access to the target server

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Related Projects

- [rRateLimit/server](https://github.com/rRateLimit/server) - The server component for testing rate limiting