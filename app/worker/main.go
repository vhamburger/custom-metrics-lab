package main

import (
	"context"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// globalState protected by a mutex to hold our metric value and timestamp
type globalState struct {
	mu            sync.RWMutex
	lastJobTime   time.Time
	metricValue   float64
	metricTimeout time.Duration
}

// numJobs is the custom metric we will export.
var numJobs = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "numJobs",
		Help: "The number of pending jobs in the queue as reported by the last message.",
	},
)

func init() {
	// Register the metric with Prometheus
	prometheus.MustRegister(numJobs)
}

func main() {
	log.Println("Starting worker...")

	// --- Configuration ---
	// Read configuration from environment variables
	projectID := getEnv("PROJECT_ID", "")
	if projectID == "" {
		log.Fatal("PROJECT_ID environment variable must be set")
	}

	subscriptionID := getEnv("SUBSCRIPTION_ID", "")
	if subscriptionID == "" {
		log.Fatal("SUBSCRIPTION_ID environment variable must be set")
	}

	jobDurationSec, _ := strconv.Atoi(getEnv("JOB_DURATION_SEC", "90"))
	jobDuration := time.Duration(jobDurationSec) * time.Second

	metricTimeoutSec, _ := strconv.Atoi(getEnv("METRIC_TIMEOUT_SEC", "120"))
	metricTimeout := time.Duration(metricTimeoutSec) * time.Second

	// --- Global State ---
	// This state tracks when we last processed a job.
	state := &globalState{
		lastJobTime:   time.Now(), // Initialize to now
		metricValue:   0,
		metricTimeout: metricTimeout,
	}

	// --- Start Metrics Server ---
	// This goroutine serves the /metrics endpoint
	go func() {
		log.Println("Starting metrics server on :8080")
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Metrics server failed: %v", err)
		}
	}()

	// --- Start Metric Updater ---
	// This goroutine is responsible for setting the metric to 0
	// if we haven't received a job in a while (metricTimeout).
	go state.metricUpdater()

	// --- Start Pub/Sub Client ---
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create pubsub client: %v", err)
	}
	defer client.Close()

	log.Printf("Listening to subscription '%s'...", subscriptionID)
	log.Printf("Config: Job Duration: %v, Metric Timeout: %v", jobDuration, metricTimeout)

	// --- Start Message Receiver ---
	sub := client.Subscription(subscriptionID)
	// CRITICAL: This ensures the pod only ever works on one message at a time.
	sub.ReceiveSettings.MaxOutstandingMessages = 1

	// Receive blocks until the context is cancelled.
	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		log.Println("Received message!")

		// 1. Parse the "numJobs" attribute from the message
		jobValStr := msg.Attributes["numJobs"]
		jobVal, err := strconv.ParseFloat(jobValStr, 64)
		if err != nil {
			log.Printf("Warning: 'numJobs' attribute missing or invalid: %v", err)
			jobVal = 1 // Default to 1 if missing
		}

		// 2. Update global state and metric
		state.updateMetric(jobVal)
		log.Printf("Set numJobs metric to %.0f", jobVal)

		// 3. Simulate the long-running, low-CPU work
		log.Printf("Starting work (simulated duration: %v)...", jobDuration)
		simulateWork(jobDuration)
		log.Println("Work finished.")

		// 4. Acknowledge the message
		// This tells Pub/Sub we are done, and the client is free
		// to pull the next message (respecting MaxOutstandingMessages=1).
		msg.Ack()
	})

	if err != nil {
		log.Fatalf("Pub/Sub Receive error: %v", err)
	}
}

// simulateWork performs a task that takes time but is not 100% CPU-bound.
// This is key to showing why CPU scaling is not effective.
func simulateWork(duration time.Duration) {
	startTime := time.Now()
	for time.Since(startTime) < duration {
		// Perform some trivial calculations to generate a *little* CPU load
		for i := 0; i < 1000000; i++ {
			_ = math.Sqrt(float64(i))
		}
		// Sleep to stretch the job's duration without maxing out the CPU
		time.Sleep(50 * time.Millisecond)
	}
}

// updateMetric safely updates the global state and the Prometheus gauge.
func (s *globalState) updateMetric(value float64) {
	s.mu.Lock()
	s.lastJobTime = time.Now()
	s.metricValue = value
	s.mu.Unlock()
	numJobs.Set(value)
}

// metricUpdater runs in a loop, checking if the last job is stale.
// If it is, it sets the metric to 0 to allow the HPA to scale down.
func (s *globalState) metricUpdater() {
	// Check every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		lastJob := s.lastJobTime
		timeout := s.metricTimeout
		s.mu.RUnlock()

		if time.Since(lastJob) > timeout {
			log.Println("No jobs received in timeout period. Setting numJobs metric to 0.")
			numJobs.Set(0)
		}
	}
}

// getEnv is a helper to read an env var with a fallback.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

