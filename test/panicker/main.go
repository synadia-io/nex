package main

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// Statically compile this and deploy it to test situations when we want to see
// how the system responds to a workload crash
func main() {
	fmt.Println("Everything is fine. Nothing to see here. Please disperse.")

	r := rand.Intn(5) + 5
	time.Sleep(time.Duration(r) * time.Second)

	err := errors.New("something terrible has happened")
	fmt.Printf("failure: %s\n", err)

	panic(err)
}
