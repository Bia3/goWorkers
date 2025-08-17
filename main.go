package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
	"workerPools/workers"
)

var MaxWorkers = 10
var MaxRetries = 3
var MyWorkers = workers.NewQueue(MaxWorkers, MaxRetries)
var totalSequentialDuration time.Duration

func processX(x int) {
	r := rand.Intn(2000)
	dur := time.Duration(r) * time.Millisecond
	time.Sleep(dur)
	fmt.Printf("Process %s: processing time: %v\n", strconv.Itoa(x), dur)
	totalSequentialDuration = totalSequentialDuration + dur
}

func main() {
	go MyWorkers.RunWorkers()

	startTime := time.Now()

	for i := 0; i < 100; i++ {
		//r := rand.Intn(100)
		//time.Sleep(time.Duration(r) * time.Millisecond)
		MyWorkers.NewItem(func() bool {
			processX(i + 1)
			return true
		})
	}

	l := MyWorkers.Len()
	rp := MyWorkers.RemainingProcesses

	fmt.Println("Pool Size:", l)

	//Wait for the pool to be clear
	for rp > 0 {
		rp = MyWorkers.RemainingProcesses
	}

	fmt.Println("Done Pool Size:", MyWorkers.Len())
	fmt.Println("Total Sequential Duration:", totalSequentialDuration)
	parDur := time.Since(startTime)
	fmt.Println("Total Parallel Duration:", parDur)
	fmt.Println("Total Time Saved:", totalSequentialDuration-parDur)
}
