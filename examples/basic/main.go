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
	fmt.Println("Checking server health...")
	if err := client.HealthCheck(ctx); err != nil {
		log.Fatal("Server is not healthy:", err)
	}
	fmt.Println("Server is healthy!")

	// Add messages
	fmt.Println("\nAdding messages to queue...")
	for i := 1; i <= 5; i++ {
		message, err := client.AddMessage(ctx, fmt.Sprintf("Message #%d", i))
		if err != nil {
			log.Printf("Failed to add message: %v", err)
			continue
		}
		fmt.Printf("Added message: %s\n", message.ID)
	}

	// Get and process messages
	fmt.Println("\nRetrieving messages from queue...")
	messages, err := client.GetMessages(ctx, 3)
	if err != nil {
		log.Fatal("Failed to get messages:", err)
	}

	fmt.Printf("Retrieved %d messages\n", len(messages))
	for _, msg := range messages {
		fmt.Printf("  - ID: %s, Body: %s, State: %s, Retries: %d\n",
			msg.ID, msg.Body, msg.State, msg.RetryCount)
	}

	// Simulate processing failure and retry
	if len(messages) > 0 {
		fmt.Printf("\nSimulating failure for message %s...\n", messages[0].ID)
		if err := client.RetryMessage(ctx, messages[0].ID); err != nil {
			log.Printf("Failed to retry message: %v", err)
		} else {
			fmt.Println("Message returned to queue for retry")
		}

		// Delete successfully processed messages
		var idsToDelete []string
		for i := 1; i < len(messages); i++ {
			idsToDelete = append(idsToDelete, messages[i].ID)
		}

		if len(idsToDelete) > 0 {
			fmt.Printf("\nDeleting %d processed messages...\n", len(idsToDelete))
			if err := client.DeleteMessages(ctx, idsToDelete); err != nil {
				log.Printf("Failed to delete messages: %v", err)
			} else {
				fmt.Println("Messages deleted successfully")
			}
		}
	}

	// Check remaining messages
	fmt.Println("\nChecking remaining messages...")
	remaining, err := client.GetMessages(ctx, 10)
	if err != nil {
		log.Printf("Failed to get remaining messages: %v", err)
	} else {
		fmt.Printf("Remaining messages in queue: %d\n", len(remaining))
	}

	// Purge queue
	fmt.Println("\nPurging queue...")
	if err := client.PurgeQueue(ctx); err != nil {
		log.Printf("Failed to purge queue: %v", err)
	} else {
		fmt.Println("Queue purged successfully")
	}
}

