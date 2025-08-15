package main

import (
    "awesomeProject1/workers"
    "fmt"
    "math/rand"
    "strconv"
    "time"
)

var MyWorkers = workers.NewQueue()

func processX(x int) {
    r := rand.Intn(500)
    time.Sleep(time.Duration(r) * time.Millisecond)
    fmt.Println("Process", strconv.Itoa(x))
}

func main() {
    go MyWorkers.RunWorkers()

    for i := 0; i < 100; i++ {
        r := rand.Intn(100)
        time.Sleep(time.Duration(r) * time.Millisecond)
        MyWorkers.Push(workers.Item{Name: "process " + strconv.Itoa(i+1), Function: func() { processX(i + 1) }})
    }

    l := MyWorkers.Len()

    fmt.Println("Pool Size:", l)

    //Wait for the pool to be clear
    for l > 0 {
        time.Sleep(100 * time.Millisecond)
        l = MyWorkers.Len()
    }
    time.Sleep(500 * time.Millisecond)

    fmt.Println("Done Pool Size:", MyWorkers.Len())
}
