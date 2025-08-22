# TLQ Client for Go

A lightweight, efficient Go client library for [TLQ (Tiny Little Queue)](https://github.com/skyaktech/tlq) - a simple, in-memory message queue for development and testing.

## Features

- Simple and intuitive API
- Automatic retry with exponential backoff
- Context-aware operations for proper cancellation
- Configurable timeout and retry settings
- Zero external dependencies (uses only Go standard library)
- Thread-safe operations
- Comprehensive error handling

## Installation

```bash
go get github.com/skyaktech/tlq-client-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    tlq "github.com/skyaktech/tlq-client-go"
)

func main() {
    // Create a new client with default settings
    client := tlq.NewClient()
    
    ctx := context.Background()
    
    // Check server health
    if err := client.HealthCheck(ctx); err != nil {
        log.Fatal("Server is not healthy:", err)
    }
    
    // Add a message
    message, err := client.AddMessage(ctx, "Hello, TLQ!")
    if err != nil {
        log.Fatal("Failed to add message:", err)
    }
    fmt.Printf("Added message: %s\n", message.ID)
    
    // Get messages
    messages, err := client.GetMessages(ctx, 1)
    if err != nil {
        log.Fatal("Failed to get messages:", err)
    }
    
    for _, msg := range messages {
        fmt.Printf("Got message: %s - %s\n", msg.ID, msg.Body)
        
        // Delete the message after processing
        if err := client.DeleteMessage(ctx, msg.ID); err != nil {
            log.Printf("Failed to delete message: %v", err)
        }
    }
}
```

## Configuration

The client can be configured using functional options:

```go
client := tlq.NewClient(
    tlq.WithHost("custom.host"),
    tlq.WithPort(8080),
    tlq.WithTimeout(60 * time.Second),
    tlq.WithMaxRetries(5),
    tlq.WithRetryDelay(200 * time.Millisecond),
)
```

### Available Options

- `WithHost(host string)` - Set the TLQ server host (default: "localhost")
- `WithPort(port int)` - Set the TLQ server port (default: 1337)
- `WithTimeout(timeout time.Duration)` - Set request timeout (default: 30s)
- `WithMaxRetries(maxRetries int)` - Set maximum retry attempts (default: 3)
- `WithRetryDelay(delay time.Duration)` - Set base retry delay (default: 100ms)
- `WithHTTPClient(client *http.Client)` - Use a custom HTTP client

## API Reference

### Client Methods

#### NewClient(opts ...Option) *Client
Creates a new TLQ client with the specified options.

#### HealthCheck(ctx context.Context) error
Checks if the TLQ server is healthy and responsive.

#### AddMessage(ctx context.Context, body string) (*Message, error)
Adds a new message to the queue. Returns the created message with its ID.
- Message body is limited to 64KB

#### GetMessages(ctx context.Context, count int) ([]*Message, error)
Retrieves up to `count` messages from the queue.

#### DeleteMessage(ctx context.Context, messageID string) error
Deletes a single message from the queue.

#### DeleteMessages(ctx context.Context, messageIDs []string) error
Deletes multiple messages from the queue.

#### RetryMessage(ctx context.Context, messageID string) error
Returns a single message to the queue for retry.

#### RetryMessages(ctx context.Context, messageIDs []string) error
Returns multiple messages to the queue for retry.

#### PurgeQueue(ctx context.Context) error
Removes all messages from the queue.

### Message Structure

```go
type Message struct {
    ID         string `json:"id"`          // UUID v7 message identifier
    Body       string `json:"body"`        // Message content
    State      string `json:"state"`       // Message state (Ready, Processing, etc.)
    RetryCount int    `json:"retry_count"` // Number of retry attempts
}
```

## Advanced Usage

### Worker Pattern

```go
func worker(ctx context.Context, client *tlq.Client, workerID int) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            messages, err := client.GetMessages(ctx, 10)
            if err != nil {
                log.Printf("Worker %d: Failed to get messages: %v", workerID, err)
                time.Sleep(time.Second)
                continue
            }
            
            for _, msg := range messages {
                // Process message
                if err := processMessage(msg); err != nil {
                    // Retry on failure
                    client.RetryMessage(ctx, msg.ID)
                } else {
                    // Delete on success
                    client.DeleteMessage(ctx, msg.ID)
                }
            }
            
            if len(messages) == 0 {
                time.Sleep(100 * time.Millisecond)
            }
        }
    }
}
```

### Batch Processing

```go
func processBatch(ctx context.Context, client *tlq.Client) error {
    messages, err := client.GetMessages(ctx, 100)
    if err != nil {
        return fmt.Errorf("failed to get messages: %w", err)
    }
    
    var successIDs, failedIDs []string
    
    for _, msg := range messages {
        if err := processMessage(msg); err != nil {
            failedIDs = append(failedIDs, msg.ID)
        } else {
            successIDs = append(successIDs, msg.ID)
        }
    }
    
    // Delete successful messages
    if len(successIDs) > 0 {
        if err := client.DeleteMessages(ctx, successIDs); err != nil {
            return fmt.Errorf("failed to delete messages: %w", err)
        }
    }
    
    // Retry failed messages
    if len(failedIDs) > 0 {
        if err := client.RetryMessages(ctx, failedIDs); err != nil {
            return fmt.Errorf("failed to retry messages: %w", err)
        }
    }
    
    return nil
}
```

### Graceful Shutdown

```go
func main() {
    client := tlq.NewClient()
    ctx, cancel := context.WithCancel(context.Background())
    
    // Handle shutdown signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("Shutting down...")
        cancel()
    }()
    
    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            worker(ctx, client, id)
        }(i)
    }
    
    wg.Wait()
    log.Println("All workers stopped")
}
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...
```

## Requirements

- Go 1.18 or higher
- TLQ server running (default: localhost:1337)

## TLQ Server Installation

Install TLQ using Cargo:

```bash
cargo install tlq
tlq
```

Or run with Docker:

```bash
docker run -p 1337:1337 ghcr.io/skyaktech/tlq:latest
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Related Projects

- [TLQ Server](https://github.com/skyaktech/tlq) - The TLQ server implementation
- [TLQ Client for Node.js](https://github.com/skyaktech/tlq-client-node) - Node.js client library
- [TLQ Client for Python](https://github.com/skyaktech/tlq-client-py) - Python client library

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/skyaktech/tlq-client-go).