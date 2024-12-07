package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

const (
	NUM_EVENTS       = 100000
	BATCH_SIZE       = 10 // Reduced batch size
	REQUESTS_PER_SEC = 1  // Rate limit: requests per second
	MAX_RETRIES      = 1   // Maximum number of retries for failed requests
	INITIAL_BACKOFF  = 100 // Initial backoff in milliseconds
	// API_ENDPOINT     = "https://api-dev.cloud.flexprice.io/v1/events"
	API_ENDPOINT    = "http://localhost:8080/v1/events/ingest"
	TIMEOUT_SECONDS = 5
)

type BatchResult struct {
	BatchNumber int
	LastEventID string
	StartTime   time.Time
	EndTime     time.Time
	EventCount  int
}

// generateEvent creates a random event with varying properties
func generateEvent(index int) dto.IngestEventRequest {
	sources := []string{"web", "mobile", "api", "backend"}
	// eventTypes := []string{"api_call", "page_view", "button_click", "form_submit"}
	eventTypes := []string{"gpu_time"}

	return dto.IngestEventRequest{
		EventID:            uuid.New().String(),
		EventName:          eventTypes[index%len(eventTypes)],
		ExternalCustomerID: fmt.Sprintf("cus_loadtest_%d", index%100), // 100 different customers
		Source:             sources[index%len(sources)],
		Timestamp:          time.Now().Add(-time.Duration(index*2) * time.Second),
		Properties: map[string]interface{}{
			"bytes_transferred": 100 + (index % 1000),
			"duration_ms":       50 + (index % 200),
			"status_code":       200 + (index%3)*100, // 200, 300, 400
			"test_group":        fmt.Sprintf("group_%d", index%10),
		},
	}
}

func ingestEvent(event dto.IngestEventRequest, limiter *rate.Limiter, wg *sync.WaitGroup, results chan<- time.Duration, errors chan<- error) {
	defer wg.Done()

	// Wait for rate limiter
	err := limiter.Wait(context.Background())
	if err != nil {
		errors <- fmt.Errorf("rate limiter error: %v", err)
		return
	}

	start := time.Now()
	retryCount := 0
	backoff := INITIAL_BACKOFF

	for retryCount <= MAX_RETRIES {
		jsonData, err := json.Marshal(event)
		if err != nil {
			errors <- fmt.Errorf("marshal error: %v", err)
			return
		}

		client := &http.Client{
			Timeout: time.Second * TIMEOUT_SECONDS,
		}

		resp, err := client.Post(API_ENDPOINT, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			if retryCount == MAX_RETRIES {
				errors <- fmt.Errorf("request error after %d retries: %v", MAX_RETRIES, err)
				return
			}
			retryCount++
			time.Sleep(time.Duration(backoff) * time.Millisecond)
			backoff *= 2 // Exponential backoff
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted {
			duration := time.Since(start)
			results <- duration
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusInternalServerError {
			if retryCount == MAX_RETRIES {
				errors <- fmt.Errorf("unexpected status after %d retries: %d", MAX_RETRIES, resp.StatusCode)
				return
			}
			retryCount++
			time.Sleep(time.Duration(backoff) * time.Millisecond)
			backoff *= 2 // Exponential backoff
			continue
		}

		errors <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
		return
	}
}

func SeedEventsClickhouse() {
	logger, err := logger.NewLogger(config.GetDefaultConfig())
	if err != nil {
		log.Fatalf("Error creating logger: %v", err)
	}

	logger.Info("Starting load test...")
	logger.Infof("Sending %d events in batches of %d with rate limit of %d req/s",
		NUM_EVENTS, BATCH_SIZE, REQUESTS_PER_SEC)

	// Create rate limiter
	limiter := rate.NewLimiter(rate.Limit(REQUESTS_PER_SEC), 1)

	var wg sync.WaitGroup
	results := make(chan time.Duration, NUM_EVENTS)
	errors := make(chan error, NUM_EVENTS)

	// Track metrics
	start := time.Now()
	successCount := 0
	errorCount := 0
	batches := make([]BatchResult, 0)

	// Process in batches
	for i := 0; i < NUM_EVENTS; i += BATCH_SIZE {
		batchStart := time.Now()
		var lastEventID string

		batchSize := BATCH_SIZE
		if i+BATCH_SIZE > NUM_EVENTS {
			batchSize = NUM_EVENTS - i
		}

		// Launch batch of requests
		for j := 0; j < batchSize; j++ {
			event := generateEvent(i + j)
			lastEventID = event.EventID
			wg.Add(1)
			go ingestEvent(event, limiter, &wg, results, errors)
		}

		// Wait for batch to complete
		wg.Wait()

		batch := BatchResult{
			BatchNumber: i/BATCH_SIZE + 1,
			LastEventID: lastEventID,
			StartTime:   batchStart,
			EndTime:     time.Now(),
			EventCount:  batchSize,
		}
		batches = append(batches, batch)

		logger.Infof("Processed batch %d: %d/%d events in %v seconds. Last Event ID: %s",
			batch.BatchNumber, i+batchSize, NUM_EVENTS, batch.EndTime.Sub(batch.StartTime).Seconds(), lastEventID)

		// Add small delay between batches
		time.Sleep(time.Second)
	}

	// Close channels
	close(results)
	close(errors)

	// Calculate metrics
	var totalDuration time.Duration
	var maxDuration time.Duration
	var minDuration = time.Hour // Start with a large value

	for duration := range results {
		successCount++
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
		if duration < minDuration {
			minDuration = duration
		}
	}

	for err := range errors {
		errorCount++
		logger.Errorf("Error: %v", err)
	}

	// Print results
	totalTime := time.Since(start)
	avgDuration := totalDuration / time.Duration(successCount)

	logger.Info("Load test completed!")
	logger.Info("Results:")
	logger.Infof("Total Time: %v", totalTime)
	logger.Infof("Successful Requests: %d", successCount)
	logger.Infof("Failed Requests: %d", errorCount)
	logger.Infof("Average Request Duration: %v", avgDuration)
	logger.Infof("Min Request Duration: %v", minDuration)
	logger.Infof("Max Request Duration: %v", maxDuration)
	logger.Infof("Requests per Second: %.2f", float64(successCount)/totalTime.Seconds())

	// Print batch information
	logger.Info("\nBatch Details:")
	for _, batch := range batches {
		logger.Infof("Batch %d: Events=%d, Duration=%v, Last Event ID=%s",
			batch.BatchNumber,
			batch.EventCount,
			batch.EndTime.Sub(batch.StartTime),
			batch.LastEventID)
	}
}
