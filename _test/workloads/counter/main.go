package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"time"
)

var (
	counter     int = 0
	maxCount    int
	panicChance float64
)

func main() {
	flag.IntVar(&maxCount, "max", -1, "max count then exit 0")
	flag.Float64Var(&panicChance, "panic", 0, "panic chance X% of the time; inputs need to be between 0-1")
	flag.Parse()

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	go func() {
		<-exit
		fmt.Fprintf(os.Stdout, "Exiting...\n")
		os.Exit(0)
	}()

	for range time.Tick(time.Second) {
		if panicChance > 0 {
			if rand.Float64() < panicChance {
				panic("simulated panic")
			}
		}
		if counter == maxCount {
			fmt.Fprintf(os.Stdout, "Counter reached max count: %d\n", maxCount)
			os.Exit(0)
		}

		fmt.Fprintf(os.Stdout, "Counter: %d\n", counter)
		counter++
	}
}
