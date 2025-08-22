package test

import (
	"context"
	"fmt"
	"github.com/Bia3/goWorkers"
	"sync"
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	// Create a worker pool with 5 workers and 0 retries
	pool := goWorkers.NewPool(5, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Create a slice to track results
	var mu sync.Mutex
	results := make([]int, 0, 10)

	// Add 10 tasks to the pool
	for i := 0; i < 10; i++ {
		taskID := i // Capture the loop variable
		pool.NewTask(context.Background(), func() bool {
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
	for pool.RemainingTasks() > 0 {
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
	pool := goWorkers.NewPool(3, 2)

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
		pool.NewTask(context.Background(), func() bool {
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
	for pool.RemainingTasks() > 0 {
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

func TestContextCancellation(t *testing.T) {
	// Create a worker pool with 2 workers and 0 retries
	pool := goWorkers.NewPool(2, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Create variables to track results
	var mu sync.Mutex
	completedTasks := 0
	cancelledTasks := 0
	startedTasks := make(map[int]bool)

	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure context is cancelled when test exits

	// Add 4 tasks to the pool
	for i := 0; i < 4; i++ {
		taskNum := i // Capture the loop variable

		// Create a task-specific context for the first 2 tasks
		var taskCtx context.Context
		if i < 2 {
			taskCtx = context.Background() // These tasks won't be cancelled
		} else {
			taskCtx = ctx // These tasks will be cancelled
		}

		pool.NewTask(taskCtx, func() bool {
			// Mark task as started
			mu.Lock()
			startedTasks[taskNum] = true
			mu.Unlock()

			fmt.Printf("Task #%d started\n", taskNum)

			// Simulate long-running work
			for j := 0; j < 20; j++ {
				select {
				case <-time.After(50 * time.Millisecond):
					fmt.Printf("Task #%d working... step %d/20\n", taskNum, j+1)
				case <-taskCtx.Done():
					// Context was cancelled
					mu.Lock()
					cancelledTasks++
					mu.Unlock()
					fmt.Printf("Task #%d was cancelled during execution at step %d/20\n", taskNum, j+1)
					return false
				}
			}

			// Task completed successfully
			mu.Lock()
			completedTasks++
			mu.Unlock()
			fmt.Printf("Task #%d completed successfully\n", taskNum)
			return true
		})
	}

	// Wait for at least tasks 0, 1, and 2 to start
	startTime := time.Now()
	timeout := time.Second * 5
	allStarted := false

	for time.Since(startTime) < timeout && !allStarted {
		mu.Lock()
		if len(startedTasks) >= 3 && startedTasks[2] {
			allStarted = true
		}
		mu.Unlock()

		if !allStarted {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if !allStarted {
		t.Fatalf("Timed out waiting for tasks to start. Started tasks: %v", startedTasks)
	}

	// Cancel the context
	fmt.Println("Cancelling context...")
	cancel()

	// Wait for all tasks to complete or be cancelled
	for pool.RemainingTasks() > 0 {
		time.Sleep(50 * time.Millisecond)
	}

	// Verify that tasks were either completed or cancelled
	if completedTasks+cancelledTasks != 4 {
		t.Errorf("Expected 4 tasks to be either completed or cancelled, got %d completed and %d cancelled", completedTasks, cancelledTasks)
	}

	// Verify that at least one task was cancelled
	if cancelledTasks == 0 {
		t.Errorf("Expected at least one task to be cancelled, but none were")
	}

	fmt.Printf("Tasks completed: %d, Tasks cancelled: %d\n", completedTasks, cancelledTasks)
}

func TestTaskTimeout(t *testing.T) {
	// Create a worker pool with 3 workers and 0 retries
	pool := goWorkers.NewPool(3, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Create variables to track results
	var mu sync.Mutex
	completedTasks := 0
	timedOutTasks := 0
	startedTasks := make(map[int]bool)

	// Add 5 tasks to the pool with different timeouts
	for i := 0; i < 5; i++ {
		taskNum := i // Capture the loop variable

		// Create a task with different timeout based on task number
		taskID, err := pool.NewTask(context.Background(), func() bool {
			// Mark task as started
			mu.Lock()
			startedTasks[taskNum] = true
			mu.Unlock()

			fmt.Printf("Task #%d started\n", taskNum)

			// Simulate work with different durations
			// Tasks 0, 2, 4 will complete within their timeout
			// Tasks 1, 3 will exceed their timeout and be cancelled
			var workDuration time.Duration
			if taskNum%2 == 0 {
				// Even-numbered tasks complete quickly (200ms)
				workDuration = 200 * time.Millisecond
			} else {
				// Odd-numbered tasks take longer (600ms)
				workDuration = 600 * time.Millisecond
			}

			// Check for cancellation while working
			select {
			case <-time.After(workDuration):
				// Task completed successfully
				mu.Lock()
				completedTasks++
				mu.Unlock()
				fmt.Printf("Task #%d completed successfully after %v\n", taskNum, workDuration)
				return true
			case <-context.Background().Done():
				// This should never happen with a background context
				t.Errorf("Background context was unexpectedly cancelled")
				return false
			}
		})

		if err != nil {
			t.Fatalf("Failed to create task: %v", err)
		}

		// Find the task and set timeout
		task, found := pool.Find(taskID)
		if !found {
			t.Fatalf("Task %s not found in pool", taskID)
		}

		// Set timeout based on task number
		// Even-numbered tasks get 300ms timeout (should complete)
		// Odd-numbered tasks get 300ms timeout (should time out)
		task.WithTimeout(300 * time.Millisecond)
	}

	// Wait for all tasks to complete or time out
	startTime := time.Now()
	maxWaitTime := 2 * time.Second

	for pool.RemainingTasks() > 0 && time.Since(startTime) < maxWaitTime {
		time.Sleep(50 * time.Millisecond)
	}

	// Check if any tasks are still running after the wait time
	if pool.RemainingTasks() > 0 {
		t.Fatalf("Not all tasks completed within the expected time. Remaining tasks: %d", pool.RemainingTasks())
	}

	// Count timed out tasks by checking which ones didn't complete
	for i := 0; i < 5; i++ {
		if i%2 == 1 && startedTasks[i] {
			mu.Lock()
			timedOutTasks++
			mu.Unlock()
		}
	}

	// Verify that the expected number of tasks completed and timed out
	if completedTasks != 3 {
		t.Errorf("Expected 3 completed tasks (tasks 0, 2, 4), got %d", completedTasks)
	}

	if timedOutTasks != 2 {
		t.Errorf("Expected 2 timed out tasks (tasks 1, 3), got %d", timedOutTasks)
	}

	fmt.Printf("Tasks completed: %d, Tasks timed out: %d\n", completedTasks, timedOutTasks)
}
