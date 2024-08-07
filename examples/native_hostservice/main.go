package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
)

func main() {
	server, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_SERVER")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_SERVER in environment")
		return
	}
	jwt, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_USER_JWT")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_USER_JWT in environment")
		return
	}
	seed, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_USER_SEED")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_USER_SEED in environment")
		return
	}

	// Warning: this will print your seed into logs
	fmt.Fprintf(os.Stdout, "Found environment!\nServer: %s\nJWT: %s\nSeed: %s\n", server, jwt, seed)

	ctx := context.Background()
	nc, err := nats.Connect(server, nats.UserJWTAndSeed(jwt, seed))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to NATS: %s\n", err)
		return
	}

	_, err = nc.Subscribe("fundemo", func(msg *nats.Msg) {
		fmt.Fprintf(os.Stdout, "Received message: %s\n", string(msg.Data))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to fundemo: %s\n", err)
		return
	}

	<-ctx.Done()
}
