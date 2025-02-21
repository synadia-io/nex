package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nats.go"
)

var (
	counter     int = 0
	maxCount    int
	toNats      bool
	panicChance float64
)

func main() {
	flag.IntVar(&maxCount, "max", -1, "max count then exit 0")
	flag.Float64Var(&panicChance, "panic", 0, "panic chance X% of the time; inputs need to be between 0-1")
	flag.BoolVar(&toNats, "nats", false, "send to nats")
	flag.Parse()

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	var output io.Writer = os.Stdout
	if toNats {
		url, ok := os.LookupEnv("NEX_WORKLOAD_NATS_URL")
		if !ok {
			fmt.Fprintf(os.Stderr, "NEX_WORKLOAD_NATS_URL not set\n")
			os.Exit(1)
		}
		nk, _ := os.LookupEnv("NEX_WORKLOAD_NATS_NKEY")
		b64Jwt, _ := os.LookupEnv("NEX_WORKLOAD_NATS_B64_JWT")
		jwt, err := base64.StdEncoding.DecodeString(b64Jwt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "base64.StdEncoding.DecodeString error: %v\n", err)
			os.Exit(1)
		}

		nc, err := nats.Connect(url, nats.UserJWTAndSeed(string(jwt), nk))
		if err != nil {
			fmt.Fprintf(os.Stderr, "nats.Connect error: %v\n", err)
			os.Exit(1)
		}
		defer nc.Close()
		outNats := &natsWriter{nc}
		output = io.MultiWriter(os.Stdout, outNats)
	}

	go func() {
		<-exit
		fmt.Fprintf(output, "Exiting...\n")
		os.Exit(0)
	}()

	for range time.Tick(time.Second) {
		if panicChance > 0 {
			if rand.Float64() < panicChance {
				panic("simulated panic")
			}
		}
		if counter == maxCount {
			fmt.Fprintf(output, "Counter reached max count: %d\n", maxCount)
			os.Exit(0)
		}

		fmt.Fprintf(output, "Counter: %d\n", counter)
		counter++
	}
}

type natsWriter struct {
	nc *nats.Conn
}

func (nw *natsWriter) Write(p []byte) (int, error) {
	err := nw.nc.Publish("myacct.logs", p)
	return len(p), err
}
