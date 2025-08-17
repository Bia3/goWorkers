package workers

import (
	"errors"
	"github.com/google/uuid"
	"sync"
)

type Item struct {
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

	list               map[string]*Item
	processQueue       []string //list of ID first in first out with requeue
	RemainingProcesses int
	MaxWorkers         int
	MaxRetries         int
	cl                 bool
}

func NewItem(function func() bool) *Item {
	return &Item{
		mu:         sync.Mutex{},
		function:   function,
		processing: false,
		id:         uuid.New().String(),
	}
}

func (i *Item) Processing() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.processing = true
}

func (i *Item) IsProcessing() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.processing
}

func (i *Item) PendingCancel() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.pendingCancel = true
}

func NewQueue(maxWorkers, maxRetries int) *Pool {
	return &Pool{
		wg:           sync.WaitGroup{},
		mu:           sync.Mutex{},
		list:         make(map[string]*Item),
		processQueue: make([]string, 0),
		cl:           false,
		MaxWorkers:   maxWorkers,
		MaxRetries:   maxRetries,
	}
}

func (p *Pool) NewItem(function func() bool) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.RemainingProcesses++

	item := NewItem(function)

	p.list[item.id] = item
	p.processQueue = append(p.processQueue, item.id)

	return item.id
}

func (p *Pool) Processing(item *Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item.Processing()
	p.list[item.id] = item
}

func (p *Pool) IsProcessing(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.list[id].IsProcessing()
}

func (p *Pool) Next() (*Item, error) {
	//p.mu.Lock()
	//defer p.mu.Unlock()

	//if len(p.processQueue) == 0 {
	//    return &Item{}, errors.New("no items in queue")
	//}
	item, err := p.dequeue()
	if err != nil {
		return &Item{}, err
	}
	return item, nil
}

func (p *Pool) remove(item *Item) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.RemainingProcesses--

	delete(p.list, item.id)
}

func (p *Pool) Find(id string) (*Item, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, found := p.list[id]
	return item, found
}

func (p *Pool) Cancel(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	item, found := p.Find(id)
	if !found {
		return errors.New("item not found")
	} else {
		if item.IsProcessing() {
			item.PendingCancel()
			p.list[item.id] = item
			return errors.New("item processing")
		}
		p.remove(item)
		p.removeFromQueue(item.id)
	}
	return nil
}

func (p *Pool) requeue(item *Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.processQueue = append(p.processQueue, item.id)
}

func (p *Pool) dequeue() (*Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.processQueue) == 0 {
		return &Item{}, errors.New("no items in queue")
	}
	id := p.processQueue[0]
	p.processQueue = p.processQueue[1:]
	item, found := p.list[id]
	if !found {
		return &Item{}, errors.New("item not found")
	}
	return item, nil
}

func (p *Pool) removeFromQueue(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, v := range p.processQueue {
		if v == id {
			p.processQueue = append(p.processQueue[:i], p.processQueue[i+1:]...)
		}
	}
}

func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.processQueue)
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

func (p *Pool) Process(item *Item) bool {
	p.Processing(item)
	return item.function()
}

func (p *Pool) ProcessNext() error {
	defer p.Done()

	item, nErr := p.Next()
	if nErr != nil {
		return nErr
	}
	success := p.Process(item)
	if !success {
		if item.pendingCancel {
			p.remove(item)
			p.removeFromQueue(item.id)
			return nil
		}
		if item.retryCount < p.MaxRetries {
			item.retryCount++
			p.requeue(item)
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
			err := p.ProcessNext()
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
						err := p.ProcessNext()
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
