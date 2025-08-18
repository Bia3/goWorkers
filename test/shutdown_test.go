package test

import (
	"context"
	"github.com/Bia3/goWorkers"
	"sync"
	"testing"
	"time"
)

func TestGracefulShutdown(t *testing.T) {
	// Create a worker pool with 3 workers and 0 retries
	pool := goWorkers.NewPool(3, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Create variables to track results
	var mu sync.Mutex
	completedTasks := 0
	longRunningTaskCompleted := false

	// Add 5 quick tasks to the pool
	for i := 0; i < 5; i++ {
		taskID, err := pool.NewTask(context.Background(), func() bool {
			// Simulate quick work
			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			completedTasks++
			mu.Unlock()

			return true
		})
		if err != nil {
			t.Fatalf("Failed to add task: %v", err)
		}
		t.Logf("Added quick task: %s", taskID)
	}

	// Add a long-running task
	_, err := pool.NewTask(context.Background(), func() bool {
		// Simulate long-running work
		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		longRunningTaskCompleted = true
		completedTasks++
		mu.Unlock()

		return true
	})
	if err != nil {
		t.Fatalf("Failed to add long-running task: %v", err)
	}

	// Wait a bit to let some tasks start
	time.Sleep(100 * time.Millisecond)

	// Initiate graceful shutdown
	t.Log("Initiating graceful shutdown...")
	shutdownCompleted := pool.Shutdown(1 * time.Second)

	if !shutdownCompleted {
		t.Error("Shutdown timed out")
	}

	// Verify that all tasks were completed
	if completedTasks != 6 {
		t.Errorf("Expected 6 completed tasks, got %d", completedTasks)
	}

	// Verify that the long-running task was completed
	if !longRunningTaskCompleted {
		t.Error("Long-running task was not completed")
	}

	// Try to add a new task after shutdown
	_, err = pool.NewTask(context.Background(), func() bool {
		return true
	})
	if err != goWorkers.ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}

	t.Log("Graceful shutdown test completed successfully")
}

func TestShutdownWithTimeout(t *testing.T) {
	// Create a worker pool with 1 worker and 0 retries
	pool := goWorkers.NewPool(1, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Add a task that will take longer than the shutdown timeout
	_, err := pool.NewTask(context.Background(), func() bool {
		// Simulate very long work
		time.Sleep(500 * time.Millisecond)
		return true
	})
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait a bit to let the task start
	time.Sleep(50 * time.Millisecond)

	// Initiate shutdown with a short timeout
	t.Log("Initiating shutdown with short timeout...")
	shutdownCompleted := pool.Shutdown(100 * time.Millisecond)

	// Verify that the shutdown timed out
	if shutdownCompleted {
		t.Error("Expected shutdown to timeout, but it completed")
	}

	t.Log("Shutdown timeout test completed successfully")
}
