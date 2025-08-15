package workers

import (
    "sync"
)

type Item struct {
    Name     string
    Function func()
}

type Pool struct {
    mu sync.Mutex
    wg sync.WaitGroup

    list []Item
    cl   bool
}

func NewQueue() *Pool {
    return &Pool{
        wg:   sync.WaitGroup{},
        mu:   sync.Mutex{},
        list: []Item{},
        cl:   false,
    }
}

func (p *Pool) Push(item Item) {
    p.mu.Lock()
    defer p.mu.Unlock()

    p.list = append(p.list, item)
}

func (p *Pool) Pop() Item {
    p.mu.Lock()
    defer p.mu.Unlock()

    item := p.list[0]
    p.list = p.list[1:]

    return item
}

func (p *Pool) Len() int {
    p.mu.Lock()
    defer p.mu.Unlock()
    return len(p.list)
}

func (p *Pool) Done() {
    p.wg.Done()
}

func (p *Pool) Wait() {
    p.wg.Wait()
}

func (p *Pool) Add() {
    p.wg.Add(1)
}

func (p *Pool) Process(item Item) {
    item.Function()
}

func (p *Pool) ProcessAll() {
    for _, item := range p.list {
        p.Add()
        go func() {
            p.Process(item)
            defer p.Done()
        }()
    }
    p.list = []Item{}

    p.Wait()
}

func (p *Pool) RunWorkers() {
    for !p.cl {
        l := p.Len()
        if l > 0 {
            if l > 10 {
                for i := 0; i < 10; i++ {
                    p.Add()
                    go func() {
                        p.Process(p.Pop())
                        defer p.Done()
                    }()
                }
            } else {
                p.ProcessAll()
            }
            p.Wait()
        }
    }
}
