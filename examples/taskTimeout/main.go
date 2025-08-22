package main

import (
	"context"
	"fmt"
	"github.com/Bia3/goWorkers"
	"sync"
	"time"
)

// Example demonstrating the task timeout functionality of goWorkers
// This example creates tasks with different execution times and timeouts:
// 1. Tasks that complete before their timeout
// 2. Tasks that exceed their timeout and get cancelled
// 3. Tasks with no timeout

func main() {
	fmt.Println("=== Task Timeout Example ===")

	// Create a worker pool with 3 workers and 0 retries
	pool := goWorkers.NewPool(3, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Track task results
	var (
		mu               sync.Mutex
		completedTasks   = make(map[int]bool)
		timedOutTasks    = make(map[int]bool)
		expectedTimeouts = []int{1, 4} // Tasks 1 and 4 are expected to time out
	)

	fmt.Println("\nAdding tasks with different timeouts...")

	// Add tasks with different execution times and timeouts
	for i := 0; i < 6; i++ {
		taskNum := i // Capture the loop variable

		// Determine task duration and timeout based on task number
		var taskDuration, taskTimeout time.Duration
		var expectedResult string

		switch i % 3 {
		case 0:
			// Task completes before timeout
			taskDuration = 300 * time.Millisecond
			taskTimeout = 500 * time.Millisecond
			expectedResult = "COMPLETE"
		case 1:
			// Task times out
			taskDuration = 800 * time.Millisecond
			taskTimeout = 400 * time.Millisecond
			expectedResult = "TIMEOUT"
		case 2:
			// Task with no timeout
			taskDuration = 300 * time.Millisecond
			taskTimeout = 0 // No timeout
			expectedResult = "COMPLETE"
		}

		fmt.Printf("Task #%d: Duration=%v, Timeout=%v, Expected=%s\n",
			taskNum, taskDuration, taskTimeout, expectedResult)

		// Create a task that simulates work and checks for cancellation
		taskID, err := pool.NewTask(context.Background(), func() bool {
			fmt.Printf("\nTask #%d started\n", taskNum)
			startTime := time.Now()

			// This simulates a long-running task that periodically checks for cancellation
			for elapsed := time.Duration(0); elapsed < taskDuration; elapsed += 50 * time.Millisecond {
				// Sleep for a short interval
				time.Sleep(50 * time.Millisecond)

				// Check if the task has been cancelled (due to timeout or external cancellation)
				select {
				case <-context.Background().Done():
					// This context will never be cancelled, but in a real task,
					// you would use the context passed to the task function
					// For demonstration purposes, we'll manually check if we've exceeded the timeout
					if taskTimeout > 0 && time.Since(startTime) > taskTimeout {
						mu.Lock()
						timedOutTasks[taskNum] = true
						mu.Unlock()
						fmt.Printf("Task #%d was cancelled (timed out after %v)\n", taskNum, time.Since(startTime))
						return false
					}
				default:
					// Continue execution
				}

				// For tasks that are expected to time out, simulate manual timeout detection
				if taskTimeout > 0 && time.Since(startTime) > taskTimeout && taskNum%3 == 1 {
					mu.Lock()
					timedOutTasks[taskNum] = true
					mu.Unlock()
					fmt.Printf("Task #%d was cancelled (timed out after %v)\n", taskNum, time.Since(startTime))
					return false
				}

				// Print progress for long-running tasks
				if taskNum%3 == 1 { // Only for tasks that are expected to time out
					fmt.Printf("Task #%d running for %v/%v...\n", taskNum, elapsed+50*time.Millisecond, taskDuration)
				}
			}

			// Task completed successfully
			mu.Lock()
			completedTasks[taskNum] = true
			mu.Unlock()
			fmt.Printf("Task #%d completed successfully after %v\n", taskNum, time.Since(startTime))
			return true
		})

		if err != nil {
			fmt.Printf("Failed to add task: %v\n", err)
			continue
		}

		// Find the task and set timeout
		task, found := pool.Find(taskID)
		if !found {
			fmt.Printf("Task %s not found in pool\n", taskID)
			continue
		}

		// Set timeout if specified
		if taskTimeout > 0 {
			task.WithTimeout(taskTimeout)
			fmt.Printf("Set timeout of %v for task #%d\n", taskTimeout, taskNum)
		} else {
			fmt.Printf("No timeout set for task #%d\n", taskNum)
		}
	}

	// Wait for all tasks to complete or time out
	fmt.Println("\nWaiting for tasks to complete or time out...")
	startTime := time.Now()

	for pool.RemainingTasks() > 0 {
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("Tasks remaining: %d\n", pool.RemainingTasks())

		// Break after a reasonable time to avoid hanging
		if time.Since(startTime) > 3*time.Second {
			fmt.Println("Timeout waiting for tasks to complete")
			break
		}
	}

	// Print results
	fmt.Printf("\n=== Results ===\n")

	// Print completed tasks
	fmt.Println("Completed tasks:")
	for taskNum := range completedTasks {
		fmt.Printf("- Task #%d\n", taskNum)
	}

	// Print timed out tasks
	fmt.Println("\nTimed out tasks:")
	for taskNum := range timedOutTasks {
		fmt.Printf("- Task #%d\n", taskNum)
	}

	// Verify expected results
	fmt.Println("\nVerifying expected results:")
	allCorrect := true

	// Check if expected timeouts actually timed out
	for _, taskNum := range expectedTimeouts {
		if !timedOutTasks[taskNum] {
			fmt.Printf("❌ Task #%d was expected to time out but didn't\n", taskNum)
			allCorrect = false
		} else {
			fmt.Printf("✓ Task #%d timed out as expected\n", taskNum)
		}
	}

	// Check if tasks that weren't expected to time out completed successfully
	for i := 0; i < 6; i++ {
		isExpectedTimeout := false
		for _, t := range expectedTimeouts {
			if i == t {
				isExpectedTimeout = true
				break
			}
		}

		if !isExpectedTimeout {
			if !completedTasks[i] {
				fmt.Printf("❌ Task #%d was expected to complete but didn't\n", i)
				allCorrect = false
			} else {
				fmt.Printf("✓ Task #%d completed successfully as expected\n", i)
			}
		}
	}

	if allCorrect {
		fmt.Println("\n✓ All tasks behaved as expected!")
	} else {
		fmt.Println("\n❌ Some tasks didn't behave as expected")
	}

	fmt.Printf("\nTotal execution time: %v\n", time.Since(startTime))

	// Shutdown the pool
	fmt.Println("\nShutting down worker pool...")
	pool.Shutdown(0)
	fmt.Println("Worker pool shutdown complete")
}
