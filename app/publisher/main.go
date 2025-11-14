package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub"
)

func getOrCreateTopic(ctx context.Context, client *pubsub.Client, topicID string) *pubsub.Topic {
	topic := client.Topic(topicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		log.Fatalf("Failed to check if topic exists: %v", err)
	}
	if !exists {
		topic, err = client.CreateTopic(ctx, topicID)
		if err != nil {
			log.Fatalf("Failed to create topic: %v", err)
		}
		log.Printf("Topic %s created.\n", topicID)
	}
	return topic
}

func publishBatch(ctx context.Context, client *pubsub.Client, topicID string, numJobs, workDuration int) error {
	log.Printf("Publishing %d jobs to topic %s...\n", numJobs, topicID)
	topic := getOrCreateTopic(ctx, client, topicID)
	var results []*pubsub.PublishResult

	// --- This is the change ---
	// We now send numJobs as an Attribute, not in the JSON body.
	numJobsStr := fmt.Sprintf("%d", numJobs)

	for i := 1; i <= numJobs; i++ {
		// The body just contains job-specific info
		data, err := json.Marshal(struct {
			ID       int    `json:"id"`
			Duration string `json:"duration"`
		}{
			ID:       i,
			Duration: fmt.Sprintf("%ds", workDuration),
		})
		if err != nil {
			return fmt.Errorf("json.Marshal: %v", err)
		}

		// Publish the message with the 'numJobs' attribute
		msg := &pubsub.Message{
			Data: data,
			Attributes: map[string]string{
				"numJobs": numJobsStr,
			},
		}
		results = append(results, topic.Publish(ctx, msg))
	}

	// Wait for all messages to be published
	for i, res := range results {
		id, err := res.Get(ctx)
		if err != nil {
			log.Printf("Failed to publish message %d: %v", i+1, err)
			continue
		}
		log.Printf("Published message %d; ID: %s", i+1, id)
	}
	log.Printf("Published %d messages with 'numJobs' attribute set to '%s'.\n", numJobs, numJobsStr)
	return nil
}

func purgeQueue(ctx context.Context, client *pubsub.Client, subID string) error {
	log.Printf("Purging queue for subscription %s...", subID)
	sub := client.Subscription(subID)
	err := sub.SeekToTime(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("SeekToTime: %v", err)
	}
	log.Println("Queue purged (all unacknowledged messages will be redelivered, then new messages will be processed).")
	log.Println("Note: This does not delete messages. It resets the subscription cursor.")
	log.Println("For a full purge, please use the Google Cloud Console to seek to a future timestamp or detach/reattach the subscription.")
	return nil
}

func runAutoMode(ctx context.Context, client *pubsub.Client, topicID string) error {
	log.Println("Starting 'auto' mode...")

	// Scenario:
	// 1. 9 messages, 90s each
	log.Println("--- Scenario 1: 9 Jobs ---")
	if err := publishBatch(ctx, client, topicID, 9, 90); err != nil {
		return err
	}
	log.Println("Waiting 2 minutes...")
	time.Sleep(2 * time.Minute)

	// 2. 3 messages, 90s each
	log.Println("--- Scenario 2: 3 Jobs ---")
	if err := publishBatch(ctx, client, topicID, 3, 90); err != nil {
		return err
	}
	log.Println("Waiting 1 minute...")
	time.Sleep(1 * time.Minute)

	// 3. 15 messages, 90s each (Spike)
	log.Println("--- Scenario 3: 15 Jobs (Spike) ---")
	if err := publishBatch(ctx, client, topicID, 15, 90); err != nil {
		return err
	}
	log.Println("Waiting 3 minutes...")
	time.Sleep(3 * time.Minute)

	// 4. 7 messages, 90s each
	log.Println("--- Scenario 4: 7 Jobs ---")
	if err := publishBatch(ctx, client, topicID, 7, 90); err != nil {
		return err
	}
	log.Println("Waiting 3 minutes...")
	time.Sleep(3 * time.Minute)

	// 5. Send a "DONE" message with numJobs = 0
	log.Println("--- Scenario 5: Done (0 Jobs) ---")

	// --- FIX: Get the topic before publishing ---
	topic := getOrCreateTopic(ctx, client, topicID)
	// --- End Fix ---

	msg := &pubsub.Message{
		Data: []byte("DONE"),
		Attributes: map[string]string{
			"numJobs": "0",
		},
	}
	res := topic.Publish(ctx, msg)
	_, err := res.Get(ctx)
	if err != nil {
		return fmt.Errorf("Failed to publish DONE message: %v", err)
	}

	log.Println("Auto mode finished.")
	return nil
}

func printUsage() {
	fmt.Println("Usage: go run . <command> <project_id> <topic_id> <subscription_id> [args]")
	fmt.Println("Commands:")
	fmt.Println("  publish <project_id> <topic_id> <subscription_id> <num_messages> <work_duration_sec>")
	fmt.Println("  purge   <project_id> <topic_id> <subscription_id>")
	fmt.Println("  auto    <project_id> <topic_id> <subscription_id>")
}

func main() {
	if len(os.Args) < 5 {
		printUsage()
		return
	}

	command := os.Args[1]
	projectID := os.Args[2]
	topicID := os.Args[3]
	subID := os.Args[4] // Used by purge, but good to be consistent

	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create pubsub client: %v", err)
	}
	defer client.Close()

	switch command {
	case "publish":
		if len(os.Args) != 7 {
			printUsage()
			return
		}
		numJobs, err := strconv.Atoi(os.Args[5])
		if err != nil {
			log.Fatalf("Invalid <num_messages>: %v", err)
		}
		workDuration, err := strconv.Atoi(os.Args[6])
		if err != nil {
			log.Fatalf("Invalid <work_duration_sec>: %v", err)
		}
		if err := publishBatch(ctx, client, topicID, numJobs, workDuration); err != nil {
			log.Fatalf("Failed to publish: %v", err)
		}

	case "purge":
		if err := purgeQueue(ctx, client, subID); err != nil {
			log.Fatalf("Failed to purge: %v", err)
		}

	case "auto":
		if err := runAutoMode(ctx, client, topicID); err != nil {
			log.Fatalf("Failed to run auto mode: %v", err)
		}

	default:
		log.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}



