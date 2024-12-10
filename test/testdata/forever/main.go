package main

import (
	"os"
	"os/signal"
	"time"
)

func main() {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-exit:
			return
		case <-ticker.C:
		}
	}
}
