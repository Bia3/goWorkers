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
type Task struct {
	mu sync.Mutex

	function      func() bool
	processing    bool
	pendingCancel bool
	retryCount    int
	id            string
	ctx           context.Context
	cancel        context.CancelFunc
}

// Pool represents a worker pool that processes tasks concurrently.
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
func (p *Pool) RemainingTasks() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.RemainingProcesses
}

// NewTask creates a new task with the given context and function.
// If ctx is nil, context.Background() is used.
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
func (t *Task) Processing() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processing = true
}

// IsProcessing returns whether the task is currently being processed.
func (t *Task) IsProcessing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.processing
}

// PendingCancel marks the task as pending cancellation and cancels its context.
func (t *Task) PendingCancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingCancel = true
	if t.cancel != nil {
		t.cancel()
	}
}

// IsCancelled returns whether the task has been cancelled.
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
func New(maxWorkers, maxRetries int) *Pool {
	return NewPool(maxWorkers, maxRetries)
}

// NewPool creates a new worker pool with the specified maximum number of workers and retries.
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
func NewQueue(maxWorkers, maxRetries int) *Pool {
	return NewPool(maxWorkers, maxRetries)
}

// NewTask adds a new task to the pool with the given context and function.
// It returns the ID of the created task and an error if the pool is closed.
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
func (p *Pool) IsProcessing(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	task, found := p.taskList[id]
	if !found {
		return false
	}
	return task.IsProcessing()
}

// Next returns the next task to be processed.
// It returns an error if there are no tasks in the queue.
func (p *Pool) Next() (*Task, error) {
	item, err := p.dequeue()
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Size returns the total number of tasks in the pool.
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
func (p *Pool) Find(id string) (*Task, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, found := p.taskList[id]
	return item, found
}

// Cancel cancels the task with the given ID.
// It returns an error if the task is not found or if it is currently processing.
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

// Len returns the number of tasks in the queue.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taskQueue)
}

// Done decrements the wait group counter.
func (p *Pool) Done() {
	p.wg.Done()
}

// Wait waits for all tasks to complete.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Shutdown gracefully shuts down the worker pool.
// It stops accepting new tasks and waits for all currently running tasks to complete.
// If timeout is greater than 0, it will wait for at most the specified duration for tasks to complete.
// If timeout is 0 or negative, it will wait indefinitely.
// Returns true if all tasks completed successfully, false if the timeout was reached.
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
		item.pendingCancel = true
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
		if item.pendingCancel || item.IsCancelled() {
			p.remove(item)
			p.removeFromQueue(item.id)
			return nil
		}

		// Retry the task if retries are available
		if item.retryCount < p.MaxRetries-1 {
			item.retryCount++
			p.requeue(item)
			return nil
		}
	}

	p.remove(item)
	p.removeFromQueue(item.id)
	return nil
}

// processAll processes all tasks in the queue.
func (p *Pool) processAll() {
	l := p.Len()
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
func (p *Pool) RunWorkers() {
	for {
		select {
		case <-p.shutdownCh:
			// Pool is shutting down, process remaining tasks but don't accept new ones
			p.processRemainingTasks()
			return
		default:
			// Continue normal operation
			l := p.Len()
			if l > 0 {
				if l > p.MaxWorkers {
					for i := 0; i < p.MaxWorkers; i++ {
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
	for p.Len() > 0 {
		p.processAll()
		p.Wait()
	}
}
