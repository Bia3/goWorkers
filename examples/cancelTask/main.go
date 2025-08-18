package main

import (
	"context"
	"fmt"
	"github.com/Bia3/goWorkers"
	"time"
)

var MaxWorkers = 20
var MaxRetries = 3
var MyWorkers = goWorkers.NewQueue(MaxWorkers, MaxRetries)

var attempt = 0

func processX() {
	attempt++
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("attempt %d\n", attempt)
}

func main() {
	go MyWorkers.RunWorkers()

	// Example 1: Using the Cancel method
	fmt.Println("Example 1: Using the Cancel method")
	taskID := MyWorkers.NewTask(context.Background(), func() bool {
		processX()
		return false
	})

	rp := MyWorkers.RemainingTasks()
	fmt.Println("taskID:", taskID)

	time.Sleep(75 * time.Millisecond)

	err := MyWorkers.Cancel(taskID)
	if err != nil {
		fmt.Println(err)
	}

	for rp > 0 {
		rp = MyWorkers.RemainingTasks()
		time.Sleep(50 * time.Millisecond)
	}

	// Example 2: Using context cancellation
	fmt.Println("\nExample 2: Using context cancellation")

	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())

	// Create a task with the cancellable context
	taskID2 := MyWorkers.NewTask(ctx, func() bool {
		for i := 0; i < 5; i++ {
			select {
			case <-time.After(50 * time.Millisecond):
				fmt.Printf("Task 2: Working... step %d\n", i+1)
			case <-ctx.Done():
				fmt.Println("Task 2: Context cancelled, stopping work")
				return false
			}
		}
		fmt.Println("Task 2: Work completed successfully")
		return true
	})

	fmt.Println("taskID2:", taskID2)

	// Wait a bit to let the task start
	time.Sleep(125 * time.Millisecond)

	// Cancel the context
	fmt.Println("Cancelling context...")
	cancel()

	// Wait for the task to complete or be cancelled
	rp = MyWorkers.RemainingTasks()
	for rp > 0 {
		rp = MyWorkers.RemainingTasks()
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("All tasks completed or cancelled")
}
