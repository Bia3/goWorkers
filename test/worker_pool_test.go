package test

import (
	"fmt"
	"github.com/Bia3/goWorkers"
	"sync"
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	// Create a worker pool with 5 workers and 0 retries
	pool := goWorkers.NewQueue(5, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Create a slice to track results
	var mu sync.Mutex
	results := make([]int, 0, 10)

	// Add 10 tasks to the pool
	for i := 0; i < 10; i++ {
		taskID := i // Capture the loop variable
		pool.NewTask(func() bool {
			// Simulate work
			time.Sleep(100 * time.Millisecond)

			// Record the result
			mu.Lock()
			results = append(results, taskID)
			mu.Unlock()

			return true
		})
	}

	// Wait for all tasks to complete
	for pool.RemainingProcesses > 0 {
		time.Sleep(50 * time.Millisecond)
	}

	// Verify that all tasks were processed
	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	fmt.Println("All tasks completed successfully")
	fmt.Println("Results:", results)
}

func TestRetryMechanism(t *testing.T) {
	// Create a worker pool with 3 workers and 2 retries
	pool := goWorkers.NewQueue(3, 2)

	// Start the worker pool
	go pool.RunWorkers()

	// Create variables to track results
	var mu sync.Mutex
	successCount := 0
	failureCount := 0
	retryCount := 0

	// Add 5 tasks to the pool that will fail on first attempt but succeed on retry
	for i := 0; i < 5; i++ {
		attempts := 0
		taskNum := i // Capture the loop variable
		pool.NewTask(func() bool {
			// Simulate work
			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			defer mu.Unlock()

			attempts++

			// Fail on first attempt, succeed on retry unless task #4 then fail out
			if attempts == 1 || taskNum == 4 {
				failureCount++
				retryCount++
				if attempts >= 2 {
					fmt.Printf("Task #%d failed, will not retry\n", taskNum)
				} else {
					fmt.Printf("Task #%d failed, will retry\n", taskNum)
				}
				return false
			}

			// Succeed on retry
			successCount++
			fmt.Printf("Task #%d succeeded on retry #%d\n", taskNum, attempts-1)
			return true
		})
	}

	// Wait for all tasks to complete
	for pool.RemainingProcesses > 0 {
		time.Sleep(50 * time.Millisecond)
	}

	// Verify that all tasks were processed and retried
	if successCount != 4 {
		t.Errorf("Expected 5 successful tasks, got %d", successCount)
	}

	if failureCount != 6 {
		t.Errorf("Expected 6 failed attempts, got %d", failureCount)
	}

	if retryCount != 6 {
		t.Errorf("Expected 6 retries, got %d", retryCount)
	}

	fmt.Println("All tasks completed successfully after retries")
}
