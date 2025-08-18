package main

import (
	"context"
	"fmt"
	"github.com/Bia3/goWorkers"
	"sync/atomic"
	"time"
)

func main() {
	// Create a worker pool with 5 workers and 0 retries
	pool := goWorkers.NewPool(5, 0)
	fmt.Println("Worker pool created with 5 workers")

	// Start the worker pool
	go pool.RunWorkers()
	fmt.Println("Worker pool started")

	// Counter for completed tasks
	var completedTasks int32

	// Add 10 tasks with varying durations
	for i := 0; i < 10; i++ {
		taskNum := i // Capture the loop variable
		duration := 100 + time.Duration(taskNum*50)*time.Millisecond

		taskID, err := pool.NewTask(context.Background(), func() bool {
			// Simulate work with varying duration
			fmt.Printf("Task %d: Starting, will take %v\n", taskNum, duration)
			time.Sleep(duration)

			// Mark task as completed
			atomic.AddInt32(&completedTasks, 1)

			fmt.Printf("Task %d: Completed after %v\n", taskNum, duration)
			return true
		})

		if err != nil {
			fmt.Printf("Failed to add task %d: %v\n", i, err)
		} else {
			fmt.Printf("Added task %d with ID: %s\n", i, taskID)
		}
	}

	// Wait a bit to let some tasks start
	time.Sleep(200 * time.Millisecond)

	// Initiate graceful shutdown
	fmt.Println("\n--- Initiating graceful shutdown ---")
	shutdownStart := time.Now()

	// Shutdown with a timeout of 2 seconds
	shutdownCompleted := pool.Shutdown(2 * time.Second)

	shutdownDuration := time.Since(shutdownStart)
	fmt.Printf("Shutdown completed in %v (timeout: %v)\n", shutdownDuration, shutdownCompleted)

	// Check how many tasks were completed
	fmt.Printf("Completed tasks: %d/10\n", atomic.LoadInt32(&completedTasks))

	// Try to add a new task after shutdown
	fmt.Println("\n--- Attempting to add a new task after shutdown ---")
	_, err := pool.NewTask(context.Background(), func() bool {
		fmt.Println("This task should not run")
		return true
	})

	if err != nil {
		fmt.Printf("As expected, adding a task after shutdown failed: %v\n", err)
	} else {
		fmt.Println("ERROR: Was able to add a task after shutdown!")
	}

	fmt.Println("\nExample completed successfully")
}
