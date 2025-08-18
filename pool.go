package goWorkers

import (
	"errors"
	"github.com/google/uuid"
	"sync"
)

type Task struct {
	mu sync.Mutex

	function      func() bool
	processing    bool
	pendingCancel bool
	retryCount    int
	id            string
}

type Pool struct {
	mu sync.Mutex
	wg sync.WaitGroup

	taskList           map[string]*Task
	taskQueue          []string //taskList of ID first in first out with requeue
	RemainingProcesses int
	MaxWorkers         int
	MaxRetries         int
	cl                 bool
}

func NewTask(function func() bool) *Task {
	return &Task{
		mu:         sync.Mutex{},
		function:   function,
		processing: false,
		id:         uuid.New().String(),
	}
}

func (t *Task) Processing() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processing = true
}

func (t *Task) IsProcessing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.processing
}

func (t *Task) PendingCancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingCancel = true
}

func NewQueue(maxWorkers, maxRetries int) *Pool {
	return &Pool{
		wg:         sync.WaitGroup{},
		mu:         sync.Mutex{},
		taskList:   make(map[string]*Task),
		taskQueue:  make([]string, 0),
		cl:         false,
		MaxWorkers: maxWorkers,
		MaxRetries: maxRetries,
	}
}

func (p *Pool) NewTask(function func() bool) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.RemainingProcesses++

	item := NewTask(function)

	p.taskList[item.id] = item
	p.taskQueue = append(p.taskQueue, item.id)

	return item.id
}

func (p *Pool) processing(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item.Processing()
	p.taskList[item.id] = item
}

func (p *Pool) IsProcessing(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.taskList[id].IsProcessing()
}

func (p *Pool) Next() (*Task, error) {
	item, err := p.dequeue()
	if err != nil {
		return &Task{}, err
	}
	return item, nil
}

func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taskList)
}

func (p *Pool) remove(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.RemainingProcesses--

	delete(p.taskList, item.id)
}

func (p *Pool) Find(id string) (*Task, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, found := p.taskList[id]
	return item, found
}

func (p *Pool) Cancel(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	item, found := p.taskList[id]
	if !found {
		return errors.New("item not found")
	} else {
		if item.IsProcessing() {
			item.PendingCancel()
			p.taskList[item.id] = item
			return errors.New("item processing")
		}
		p.remove(item)
		p.removeFromQueue(item.id)
	}
	return nil
}

func (p *Pool) requeue(item *Task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taskQueue = append(p.taskQueue, item.id)
}

func (p *Pool) dequeue() (*Task, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.taskQueue) == 0 {
		return &Task{}, errors.New("no items in queue")
	}
	id := p.taskQueue[0]
	p.taskQueue = p.taskQueue[1:]
	item, found := p.taskList[id]
	if !found {
		return &Task{}, errors.New("item not found")
	}
	return item, nil
}

func (p *Pool) removeFromQueue(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, v := range p.taskQueue {
		if v == id {
			p.taskQueue = append(p.taskQueue[:i], p.taskQueue[i+1:]...)
		}
	}
}

func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.taskQueue)
}

func (p *Pool) Done() {
	p.wg.Done()
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) AddWait() {
	p.wg.Add(1)
}

func (p *Pool) process(item *Task) bool {
	p.processing(item)
	return item.function()
}

func (p *Pool) processNext() error {
	defer p.Done()

	item, nErr := p.Next()
	if nErr != nil {
		return nErr
	}
	success := p.process(item)
	if !success {
		if item.pendingCancel {
			p.remove(item)
			p.removeFromQueue(item.id)
			return nil
		}
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

func (p *Pool) RunWorkers() {
	for !p.cl {
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
		}
	}
}
