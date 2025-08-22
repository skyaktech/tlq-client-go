package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tlq "github.com/skyaktech/tlq-client-go"
)

// simulateWork simulates processing a message
func simulateWork(msg *tlq.Message) error {
	// Simulate processing time
	time.Sleep(time.Duration(100+rand.Intn(400)) * time.Millisecond)

	// Randomly fail 20% of messages
	if rand.Float32() < 0.2 {
		return fmt.Errorf("simulated processing error")
	}

	return nil
}

// worker processes messages from the queue
func worker(ctx context.Context, client *tlq.Client, workerID int, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d: Shutting down", workerID)
			return
		default:
			// Get messages from queue
			messages, err := client.GetMessages(ctx, 5)
			if err != nil {
				log.Printf("Worker %d: Failed to get messages: %v", workerID, err)
				time.Sleep(time.Second)
				continue
			}

			if len(messages) == 0 {
				// No messages available, wait before polling again
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Process each message
			for _, msg := range messages {
				log.Printf("Worker %d: Processing message %s: %s", workerID, msg.ID[:8], msg.Body)

				if err := simulateWork(msg); err != nil {
					log.Printf("Worker %d: Failed to process message %s: %v (retry count: %d)",
						workerID, msg.ID[:8], err, msg.RetryCount)

					// Return message to queue for retry
					if msg.RetryCount < 3 {
						if err := client.RetryMessage(ctx, msg.ID); err != nil {
							log.Printf("Worker %d: Failed to retry message %s: %v",
								workerID, msg.ID[:8], err)
						}
					} else {
						// Max retries exceeded, delete the message (in production, you'd move to DLQ)
						log.Printf("Worker %d: Message %s exceeded max retries, removing",
							workerID, msg.ID[:8])
						if err := client.DeleteMessage(ctx, msg.ID); err != nil {
							log.Printf("Worker %d: Failed to delete message %s: %v",
								workerID, msg.ID[:8], err)
						}
					}
				} else {
					log.Printf("Worker %d: Successfully processed message %s", workerID, msg.ID[:8])
					// Delete successfully processed message
					if err := client.DeleteMessage(ctx, msg.ID); err != nil {
						log.Printf("Worker %d: Failed to delete message %s: %v",
							workerID, msg.ID[:8], err)
					}
				}
			}
		}
	}
}

// producer adds messages to the queue
func producer(ctx context.Context, client *tlq.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	messageCount := 0
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Producer: Shutting down")
			return
		case <-ticker.C:
			messageCount++
			body := fmt.Sprintf("Task #%d created at %s", messageCount, time.Now().Format(time.RFC3339))

			msg, err := client.AddMessage(ctx, body)
			if err != nil {
				log.Printf("Producer: Failed to add message: %v", err)
			} else {
				log.Printf("Producer: Added message %s", msg.ID[:8])
			}
		}
	}
}

func main() {
	// Create client with custom configuration
	client := tlq.NewClient(
		tlq.WithTimeout(10*time.Second),
		tlq.WithMaxRetries(3),
		tlq.WithRetryDelay(200*time.Millisecond),
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check server health
	if err := client.HealthCheck(ctx); err != nil {
		log.Fatal("TLQ server is not available:", err)
	}
	log.Println("Connected to TLQ server")

	// Start with a clean queue
	if err := client.PurgeQueue(ctx); err != nil {
		log.Printf("Failed to purge queue: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start workers and producer
	var wg sync.WaitGroup
	numWorkers := 3

	// Start workers
	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go worker(ctx, client, i, &wg)
		log.Printf("Started worker %d", i)
	}

	// Start producer
	wg.Add(1)
	go producer(ctx, client, &wg)
	log.Println("Started producer")

	// Wait for shutdown signal
	<-sigChan
	log.Println("\nReceived shutdown signal, stopping...")

	// Cancel context to stop all goroutines
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Println("All workers and producer stopped. Goodbye!")
}

