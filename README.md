# goWorkers

Is a Library for managing a background pool of workers that can be configured to match the number of CPUs availible on the system and can set a maximum number of retires.

Currently, this is a work in progress and is my first library. Any help or comments are welcome.

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/Bia3/goWorkers"
    "math/rand"
    "strconv"
    "time"
)

var MaxWorkers = 20
var MaxRetries = 0
var MyWorkers = goWorkers.NewQueue(MaxWorkers, MaxRetries)
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
        MyWorkers.NewTask(func() bool {
            processX(i + 1)
            return true
        })
    }

    l := MyWorkers.Size()
    rp := MyWorkers.RemainingProcesses

    fmt.Println("Pool Size:", l)

    //Wait for the pool to be clear
    for rp > 0 {
        rp = MyWorkers.RemainingProcesses
        if int(time.Since(startTime).Milliseconds())%2000 == 0 {
            fmt.Printf("Processs remaining: %d\n  Current queue length: %d\n  Currently processing: %d\n", rp, MyWorkers.Len(), rp-MyWorkers.Len())
            time.Sleep(20 * time.Millisecond)
        }
    }

    fmt.Println("Pool Size at Completion:", MyWorkers.Size())
    fmt.Println("Total Sequential Duration:", totalSequentialDuration)
    parDur := time.Since(startTime)
    fmt.Println("Total Parallel Duration:", parDur)
    fmt.Printf("Parallel Speedup: %.2f%%\n", (1-float64(parDur)/float64(totalSequentialDuration))*100)
    fmt.Println("Total Time Saved:", totalSequentialDuration-parDur)
}
```