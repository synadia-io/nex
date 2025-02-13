package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"
)

var (
	counter  int = 0
	maxCount int = -1
)

func main() {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	if args := os.Args[1:]; len(args) > 0 {
		var err error
		maxCount, err = strconv.Atoi(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	go func() {
		<-exit
		fmt.Fprintf(os.Stdout, "Exiting...\n")
		os.Exit(0)
	}()

	for range time.Tick(time.Second) {
		if counter == maxCount {
			fmt.Fprintf(os.Stdout, "Counter reached max count: %d\n", maxCount)
			os.Exit(0)
		}

		fmt.Fprintf(os.Stdout, "Counter: %d\n", counter)
		counter++
	}
}
