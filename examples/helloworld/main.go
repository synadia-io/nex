package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx := context.Background()

	fmt.Fprintln(os.Stdout, "helloworld!")
	setupSignalHandlers()

	<-ctx.Done()
}

func setupSignalHandlers() {
	go func() {
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt || s == syscall.SIGQUIT:
				fmt.Fprintf(os.Stdout, "Caught signal [%s], requesting clean shutdown", s.String())
				os.Exit(0)

			default:
				os.Exit(0)
			}
		}
	}()
}
