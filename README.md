# goWorkers

A Go library for managing a background pool of workers that can be configured to match the number of CPUs available on the system. It provides features such as task retries, context-based cancellation, and graceful shutdown.

## Features

- Configure the number of concurrent workers
- Set maximum number of retries for failed tasks
- Cancel tasks using context
- Set timeouts for task execution
- Graceful shutdown mechanism
- Simple and easy-to-use API

Currently, this is a work in progress. Any help or comments are welcome.

## Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "github.com/Bia3/goWorkers"
    "math/rand"
    "time"
)

func main() {
    // Create a worker pool with 20 workers and 0 retries
    pool := goWorkers.NewPool(20, 0)

    // Start the worker pool
    go pool.RunWorkers()

    startTime := time.Now()
    var totalSequentialDuration time.Duration

    // Add 100 tasks to the pool
    for i := 0; i < 100; i++ {
        taskNum := i // Capture the loop variable

        // Create a task with a background context
        _, err := pool.NewTask(context.Background(), func() bool {
            // Simulate work
            r := rand.Intn(2000)
            dur := time.Duration(r) * time.Millisecond
            time.Sleep(dur)

            fmt.Printf("Process %d: processing time: %v\n", taskNum+1, dur)
            totalSequentialDuration += dur

            return true // Task succeeded
        })

        if err != nil {
            fmt.Printf("Failed to add task: %v\n", err)
        }
    }

    // Wait for all tasks to complete
    for pool.RemainingTasks() > 0 {
        time.Sleep(100 * time.Millisecond)
        fmt.Printf("Tasks remaining: %d\n", pool.RemainingTasks())
    }

    // Print statistics
    fmt.Println("Total Sequential Duration:", totalSequentialDuration)
    parDur := time.Since(startTime)
    fmt.Println("Total Parallel Duration:", parDur)
    fmt.Printf("Parallel Speedup: %.2f%%\n", (1-float64(parDur)/float64(totalSequentialDuration))*100)
    fmt.Println("Total Time Saved:", totalSequentialDuration-parDur)
}
```

## Graceful Shutdown

The library provides a graceful shutdown mechanism that allows you to stop accepting new tasks and wait for all currently running tasks to complete.

```go
// Shutdown with no timeout (wait indefinitely for tasks to complete)
pool.Shutdown(0)

// Shutdown with a timeout of 5 seconds
completed := pool.Shutdown(5 * time.Second)
if !completed {
    fmt.Println("Shutdown timed out, some tasks may not have completed")
}
```

## Task Timeouts

You can set a timeout for task execution, after which the task will be automatically cancelled if it hasn't completed.

```go
// Create a task with a background context
taskID, err := pool.NewTask(context.Background(), func() bool {
    // Simulate long-running work
    time.Sleep(2 * time.Second)
    return true
})

if err != nil {
    fmt.Printf("Failed to add task: %v\n", err)
}

// Find the task and set a timeout of 1 second
task, found := pool.Find(taskID)
if found {
    task.WithTimeout(1 * time.Second)
}

// The task will be automatically cancelled after 1 second if it hasn't completed
```

## Complete Examples

- [Simple Pool](examples/simplePool/main.go) - Basic usage of the worker pool
- [Graceful Shutdown](examples/gracefulShutdown/main.go) - How to use the graceful shutdown mechanism
- [Task Cancellation](examples/cancelTask/main.go) - How to cancel tasks using context
- [Task Timeout](examples/taskTimeout/main.go) - How to set timeouts for tasks
