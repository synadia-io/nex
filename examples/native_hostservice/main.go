package main

import (
	"fmt"
	"os"
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

	fmt.Fprintf(os.Stdout, "Found environment!\nServer: %s\nJWT: %s\nSeed: %s\n", server, jwt, seed)
}
