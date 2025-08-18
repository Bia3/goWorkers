package main

import (
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

	taskID := MyWorkers.NewTask(func() bool {
		processX()
		return false
	})

	rp := MyWorkers.RemainingProcesses
	fmt.Println("taskID:", taskID)

	time.Sleep(75 * time.Millisecond)

	err := MyWorkers.Cancel(taskID)
	if err != nil {
		fmt.Println(err)
	}

	for rp > 0 {
		rp = MyWorkers.RemainingProcesses
		time.Sleep(50 * time.Millisecond)
	}
}
