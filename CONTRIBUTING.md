# goWorkers Development Guidelines

This document provides guidelines and information for developers working on the goWorkers project.

## Build/Configuration Instructions

### Prerequisites

- Go 1.24 or later
- Git

### Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/Bia3/goWorkers.git
   cd goWorkers
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Build the project:
   ```bash
   go build
   ```

## Testing Information

### Running Tests

To run all tests:
```bash
go test ./...
```

To run a specific test:
```bash
go test -v -run TestName ./path/to/package
```

For example, to run the parallel processing test:
```bash
go test -v -run TestParallelProcessing ./test
```

### Adding New Tests

1. Create a new test file in the `test` directory with a name that describes what you're testing (e.g., `parallel_processing_test.go`).
2. Import the necessary packages, including the goWorkers package.
3. Create test functions with names starting with `Test`.
4. Use the `testing.T` parameter to report errors.

### Test Example

Here's an example of a test that demonstrates parallel processing using the worker pool:

```go
package test

import (
	"context"
	"fmt"
	"github.com/Bia3/goWorkers"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelProcessing(t *testing.T) {
	// Create a worker pool with 5 workers and 0 retries
	pool := goWorkers.NewQueue(5, 0)

	// Start the worker pool
	go pool.RunWorkers()

	// Number of tasks to process
	numTasks := 10

	// Create atomic counter to track completed tasks
	var completedTasks int32

	// Create a wait group to wait for all tasks to be added to the pool
	var wg sync.WaitGroup
	wg.Add(numTasks)

	// Add tasks to the pool
	for i := 0; i < numTasks; i++ {
		taskID := i // Capture the loop variable

		pool.NewTask(context.Background(), func() bool {
			defer wg.Done()

			// Simulate work with varying duration
			duration := 50 + time.Duration(taskID*10)*time.Millisecond
			time.Sleep(duration)

			// Mark task as completed
			atomic.AddInt32(&completedTasks, 1)

			fmt.Printf("Task %d completed after %v\n", taskID, duration)
			return true
		})
	}

	// Wait for all tasks to be added to the pool
	wg.Wait()

	// Wait for all tasks to complete
	for pool.RemainingTasks() > 0 {
		time.Sleep(50 * time.Millisecond)
	}

	// Verify that all tasks were processed
	if atomic.LoadInt32(&completedTasks) != int32(numTasks) {
		t.Errorf("Expected %d completed tasks, got %d", numTasks, atomic.LoadInt32(&completedTasks))
	}
}
```

## Additional Development Information

### Code Style

- Follow standard Go code style and conventions.
- Use `gofmt` to format your code.
- Use meaningful variable and function names.
- Add comments to explain complex logic.

### Project Structure

- `pool.go`: Contains the main implementation of the worker pool.
- `test/`: Contains test files.
- `scratch/`: Contains example code and experiments.

### Key Components

1. **Item**: Represents a task to be processed by the worker pool.
   - `function`: The function to be executed.
   - `retryCount`: The number of times the task has been retried.
   - `processing`: Whether the task is currently being processed.
   - `pendingCancel`: Whether the task has been marked for cancellation.

2. **Pool**: Represents the worker pool.
   - `list`: A map of tasks by ID.
   - `processQueue`: A queue of task IDs to be processed.
   - `MaxWorkers`: The maximum number of concurrent workers.
   - `MaxRetries`: The maximum number of retries for failed tasks.

### Known Issues

- The retry mechanism may not work as expected. When a task fails, it should be requeued and retried, but there appears to be an issue with how tasks are removed from the pool after being requeued.

### Future Improvements

- [ ] Fix the retry mechanism.
- [ ] Add more comprehensive tests.
- [ ] Add more examples demonstrating different use cases.
- [ ] Add documentation for all exported functions and types.
