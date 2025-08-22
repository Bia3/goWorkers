// Package goWorkers provides a flexible and efficient worker pool implementation for concurrent task processing.
// It allows for managing a pool of workers that can process tasks concurrently with features like
// task cancellation, retries, and graceful shutdown.
package goWorkers

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

// ErrNoTasksInQueue is returned when trying to dequeue from an empty queue
var ErrNoTasksInQueue = errors.New("no tasks in queue")

// ErrTaskNotFound is returned when a task with the specified ID is not found
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskProcessing is returned when trying to cancel a task that is currently processing
var ErrTaskProcessing = errors.New("task is processing, cancellation signal sent")

// ErrPoolClosed is returned when trying to add a task to a closed pool
var ErrPoolClosed = errors.New("worker pool is closed, not accepting new tasks")

// Task represents a unit of work to be processed by the worker pool.
// It encapsulates the function to be executed, its execution state, and context for cancellation.
// Tasks are created using the NewTask function and are typically added to a Pool for execution.
type Task struct {
	mu sync.Mutex

	function      func() bool        // The function to execute, returns true if successful
	processing    bool               // Whether the task is currently being processed
	pendingCancel bool               // Whether the task has been marked for cancellation
	retryCount    int                // The number of times the task has been retried
	id            string             // Unique identifier for the task
	ctx           context.Context    // Context for cancellation
	cancel        context.CancelFunc // Function to cancel the context
}

// Pool represents a worker pool that processes tasks concurrently.
// It manages a collection of tasks and a configurable number of worker goroutines
// that process these tasks. The pool supports features like task cancellation,
// retries for failed tasks, and graceful shutdown.
type Pool struct {
	mu sync.Mutex
	wg sync.WaitGroup

	taskList           map[string]*Task // Map of tasks by ID
	taskQueue          []string         // Queue of task IDs (FIFO with requeue capability)
	RemainingProcesses int              // Number of tasks that still need to be processed
	MaxWorkers         int              // Maximum number of concurrent workers
	MaxRetries         int              // Maximum number of retries for failed tasks
	closed             bool             // Whether the pool is closed
	shutdownCh         chan struct{}    // Channel to signal shutdown
}

// RemainingTasks returns the number of tasks that still need to be processed.
// This includes tasks that are currently being processed and tasks that are waiting in the queue.
// This method is thread-safe and can be used to monitor the progress of the worker pool.
func (p *Pool) RemainingTasks() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.RemainingProcesses
}

// NewTask creates a new task with the given context and function.
// If ctx is nil, context.Background() is used.
//
// The function parameter should return true if the task was processed successfully,
// or false if it failed and should be retried (subject to the MaxRetries limit of the Pool).
//
// The returned Task is ready to be added to a Pool for execution.
func NewTask(ctx context.Context, function func() bool) *Task {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Task{
		mu:         sync.Mutex{},
		function:   function,
		processing: false,
		id:         uuid.New().String(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Processing marks the task as being processed.
// This method is thread-safe and is typically called by the Pool when it starts processing the task.
func (t *Task) Processing() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processing = true
}

// ResetProcessing resets the processing state of the task.
// This method is thread-safe and is typically called by the Pool when it needs to requeue a task for retry.
func (t *Task) ResetProcessing() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processing = false
}

// IsProcessing returns whether the task is currently being processed.
// This method is thread-safe and can be used to check if a task is currently being executed.
func (t *Task) IsProcessing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.processing
}

// PendingCancel marks the task as pending cancellation and cancels its context.
// This method is thread-safe and is typically called by the Pool when a task is cancelled.
// It signals the task to stop execution as soon as possible by cancelling its context.
func (t *Task) PendingCancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingCancel = true
	if t.cancel != nil {
		t.cancel()
	}
}

// IsCancelled returns whether the task has been cancelled.
// This method checks if the task's context has been cancelled, which can happen
// when the PendingCancel method is called or when the parent context is cancelled.
// It is used to determine if a task should continue execution.
func (t *Task) IsCancelled() bool {
	select {
	case <-t.ctx.Done():
		return true
	default:
		return false
	}
}

// New creates a new worker pool with the specified maximum number of workers and retries.
// This is an alias for NewPool for backward compatibility.
//
// The maxWorkers parameter specifies the maximum number of concurrent worker goroutines.
// The maxRetries parameter specifies how many times a failed task will be retried.
//
// After creating a pool, call RunWorkers() to start processing tasks.
func New(maxWorkers, maxRetries int) *Pool {
	return NewPool(maxWorkers, maxRetries)
}

// NewPool creates a new worker pool with the specified maximum number of workers and retries.
//
// The maxWorkers parameter specifies the maximum number of concurrent worker goroutines.
// The maxRetries parameter specifies how many times a failed task will be retried.
//
// After creating a pool, call RunWorkers() to start processing tasks.
func NewPool(maxWorkers, maxRetries int) *Pool {
	return &Pool{
		wg:         sync.WaitGroup{},
		mu:         sync.Mutex{},
		taskList:   make(map[string]*Task),
		taskQueue:  make([]string, 0),
		closed:     false,
		MaxWorkers: maxWorkers,
		MaxRetries: maxRetries,
		shutdownCh: make(chan struct{}),
	}
}

// NewQueue creates a new worker pool with the specified maximum number of workers and retries.
// Deprecated: Use New or NewPool instead.
//
// This function is maintained for backward compatibility and will be removed in a future version.
func NewQueue(maxWorkers, maxRetries int) *Pool {
	return NewPool(maxWorkers, maxRetries)
}

// NewTask adds a new task to the pool with the given context and function.
// It returns the ID of the created task and an error if the pool is closed.
//
// The context can be used to cancel the task before or during execution.
// The function should return true if the task was processed successfully,
// or false if it failed and should be retried (subject to the MaxRetries limit).
//
// This method is thread-safe and can be called concurrently from multiple goroutines.
// If the pool is closed (after Shutdown was called), ErrPoolClosed is returned.
func (p *Pool) NewTask(ctx context.Context, function func() bool) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return "", ErrPoolClosed
	}

	p.RemainingProcesses++

	item := NewTask(ctx, function)

	p.taskList[item.id] = item
	p.taskQueue = append(p.taskQueue, item.id)

	return item.id, nil
}

// processing marks a task as being processed.
func (p *Pool) processing(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item.Processing()
	p.taskList[item.id] = item
}

// IsProcessing returns whether the task with the given ID is currently being processed.
// If the task is not found in the pool, false is returned.
//
// This method is thread-safe and can be used to check the status of a specific task.
// It's useful for monitoring long-running tasks or implementing custom task management logic.
func (p *Pool) IsProcessing(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	task, found := p.taskList[id]
	if !found {
		return false
	}
	return task.IsProcessing()
}

// Next returns the next task to be processed from the queue.
// It returns an error if there are no tasks in the queue (ErrNoTasksInQueue).
//
// This method is thread-safe and removes the task from the queue.
// It's primarily used internally by the worker pool to get the next task to process,
// but can also be used to implement custom task processing logic.
func (p *Pool) Next() (*Task, error) {
	item, err := p.dequeue()
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Size returns the total number of tasks in the pool, including both queued and processing tasks.
// This method is thread-safe and can be used to monitor the overall size of the worker pool.
//
// Note that this differs from Len(), which returns only the number of tasks in the queue,
// and from RemainingTasks(), which returns the number of tasks that still need to be processed.
func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taskList)
}

// remove removes a task from the pool.
func (p *Pool) remove(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.RemainingProcesses--

	delete(p.taskList, item.id)
}

// Find returns the task with the given ID and a boolean indicating whether it was found.
// This method is thread-safe and can be used to retrieve a specific task by its ID.
//
// It's useful for checking the status of a task or for implementing custom task management logic.
// The returned task should not be modified directly, as this could lead to race conditions.
func (p *Pool) Find(id string) (*Task, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, found := p.taskList[id]
	return item, found
}

// Cancel cancels the task with the given ID.
// It returns an error if the task is not found (ErrTaskNotFound) or if it is currently processing (ErrTaskProcessing).
//
// If the task is not currently being processed, it is removed from the pool immediately.
// If the task is being processed, it is marked for cancellation and will be removed after it completes.
// The task's context is cancelled in both cases, which allows the task function to detect cancellation.
//
// This method is thread-safe and can be used to cancel tasks that are no longer needed.
func (p *Pool) Cancel(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	item, found := p.taskList[id]
	if !found {
		return ErrTaskNotFound
	}

	// Cancel the task's context
	item.PendingCancel()
	p.taskList[item.id] = item

	// If the task is not processing, remove it immediately
	if !item.IsProcessing() {
		p.RemainingProcesses--
		delete(p.taskList, item.id)
		p.removeFromQueue(item.id)
		return nil
	}

	// If the task is processing, it will be removed after it completes
	return ErrTaskProcessing
}

// requeue adds a task back to the queue for retrying.
func (p *Pool) requeue(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taskQueue = append(p.taskQueue, item.id)
}

// dequeue removes and returns the next task from the queue.
// It returns an error if there are no tasks in the queue.
func (p *Pool) dequeue() (*Task, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.taskQueue) == 0 {
		return nil, ErrNoTasksInQueue
	}
	id := p.taskQueue[0]
	p.taskQueue = p.taskQueue[1:]
	item, found := p.taskList[id]
	if !found {
		return nil, ErrTaskNotFound
	}
	return item, nil
}

// removeFromQueue removes a task from the queue by ID.
func (p *Pool) removeFromQueue(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, v := range p.taskQueue {
		if v == id {
			p.taskQueue = append(p.taskQueue[:i], p.taskQueue[i+1:]...)
			break
		}
	}
}

// Len returns the number of tasks in the queue waiting to be processed.
// This method is thread-safe and can be used to monitor the queue length.
//
// Note that this differs from Size(), which returns the total number of tasks in the pool,
// and from RemainingTasks(), which returns the number of tasks that still need to be processed.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taskQueue)
}

// Done decrements the wait group counter.
// This method is used internally by the worker pool to signal that a task has completed processing.
// It should generally not be called directly unless you're implementing custom task processing logic.
func (p *Pool) Done() {
	p.wg.Done()
}

// Wait waits for all currently processing tasks to complete.
// This method blocks until all tasks that have been started with AddWait() have called Done().
//
// It's useful for implementing synchronization points in your application,
// such as waiting for a batch of tasks to complete before proceeding.
// Note that this only waits for tasks that have been started, not for tasks in the queue.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Shutdown gracefully shuts down the worker pool.
// It stops accepting new tasks and waits for all currently running tasks to complete.
// If timeout is greater than 0, it will wait for at most the specified duration for tasks to complete.
// If timeout is 0 or negative, it will wait indefinitely.
// Returns true if all tasks completed successfully, false if the timeout was reached.
//
// This method is thread-safe and can be called from any goroutine.
// After calling Shutdown, any attempts to add new tasks to the pool will return ErrPoolClosed.
// This method should be called when you're done with the worker pool to ensure all resources are properly released.
func (p *Pool) Shutdown(timeout time.Duration) bool {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return true
	}
	p.closed = true
	p.mu.Unlock()

	// Signal the RunWorkers method to stop
	close(p.shutdownCh)

	// If timeout is 0 or negative, wait indefinitely
	if timeout <= 0 {
		p.Wait()
		return true
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		p.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// AddWait increments the wait group counter.
// This method is used internally by the worker pool to signal that a new task is being processed.
// It should generally not be called directly unless you're implementing custom task processing logic.
//
// For each call to AddWait(), there should be a corresponding call to Done() when the task completes.
// The Wait() method will block until all tasks that have been started with AddWait() have called Done().
func (p *Pool) AddWait() {
	p.wg.Add(1)
}

// process processes a task and returns whether it was successful.
func (p *Pool) process(item *Task) bool {
	// Check if the task has been cancelled before processing
	if item.IsCancelled() {
		return false
	}

	p.processing(item)

	// Create a channel to signal task completion
	done := make(chan bool, 1)

	// Run the task function in a goroutine
	go func() {
		result := item.function()
		done <- result
	}()

	// Wait for either task completion or cancellation
	select {
	case result := <-done:
		return result
	case <-item.ctx.Done():
		// Task was cancelled during execution
		item.mu.Lock()
		item.pendingCancel = true
		item.mu.Unlock()
		// Wait for the function to complete to avoid goroutine leaks
		<-done
		return false
	}
}

// processNext processes the next task in the queue.
func (p *Pool) processNext() error {
	defer p.Done()

	item, err := p.Next()
	if err != nil {
		return err
	}

	// Check if the task has been cancelled before processing
	if item.IsCancelled() {
		p.remove(item)
		p.removeFromQueue(item.id)
		return nil
	}

	success := p.process(item)
	if !success {
		// If the task was cancelled or marked for cancellation, don't retry
		item.mu.Lock()
		pendingCancel := item.pendingCancel
		item.mu.Unlock()

		if pendingCancel || item.IsCancelled() {
			p.remove(item)
			p.removeFromQueue(item.id)
			return nil
		}

		// Retry the task if retries are available
		item.mu.Lock()
		retryCount := item.retryCount
		if retryCount < p.MaxRetries-1 {
			item.retryCount++
			item.mu.Unlock()
			item.ResetProcessing()
			p.requeue(item)
			return nil
		}
		item.mu.Unlock()
	}

	p.remove(item)
	p.removeFromQueue(item.id)
	return nil
}

// processAll processes all tasks in the queue.
func (p *Pool) processAll() {
	p.mu.Lock()
	l := len(p.taskQueue)
	p.mu.Unlock()

	for i := 0; i < l; i++ {
		p.AddWait()
		go func() {
			err := p.processNext()
			if err != nil {
				return
			}
		}()
	}
}

// RunWorkers starts the worker pool and processes tasks as they are added.
// It will continue running until Shutdown is called.
//
// This method should be called after creating a pool and before adding tasks.
// It typically runs in its own goroutine:
//
//	pool := goWorkers.NewPool(5, 3)
//	go pool.RunWorkers()
//
// The worker pool will process tasks concurrently up to the MaxWorkers limit.
// If there are more tasks than workers, the excess tasks will be queued and processed as workers become available.
// Failed tasks will be retried up to MaxRetries times.
func (p *Pool) RunWorkers() {
	for {
		select {
		case <-p.shutdownCh:
			// Pool is shutting down, process remaining tasks but don't accept new ones
			p.processRemainingTasks()
			return
		default:
			// Continue normal operation
			p.mu.Lock()
			l := len(p.taskQueue)
			maxWorkers := p.MaxWorkers
			p.mu.Unlock()

			if l > 0 {
				if l > maxWorkers {
					for i := 0; i < maxWorkers; i++ {
						p.AddWait()
						go func() {
							err := p.processNext()
							if err != nil {
								return
							}
						}()
					}
				} else {
					p.processAll()
				}
				p.Wait()
			} else {
				// No tasks to process, sleep briefly to avoid CPU spinning
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

// processRemainingTasks processes all remaining tasks in the queue.
// This is used during shutdown to ensure all tasks are completed.
func (p *Pool) processRemainingTasks() {
	// Process all remaining tasks
	for {
		p.mu.Lock()
		queueEmpty := len(p.taskQueue) == 0
		p.mu.Unlock()

		if queueEmpty {
			break
		}

		p.processAll()
		p.Wait()
	}
}
