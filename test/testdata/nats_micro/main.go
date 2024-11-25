package main

import (
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

func main() {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	url, exists := os.LookupEnv("NATS_URL")
	if !exists {
		panic("NATS_URL not set")
	}

	nc, err := nats.Connect(url)
	if err != nil {
		panic(err)
	}

	s, err := micro.AddService(nc, micro.Config{Name: "tester", Version: "0.0.0"})
	if err != nil {
		panic(err)
	}

	err = s.AddEndpoint("health", micro.HandlerFunc(func(r micro.Request) {
		_ = r.Respond([]byte("ok"))
	}))
	if err != nil {
		panic(err)
	}

	<-exit
}
