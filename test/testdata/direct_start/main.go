package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(os.Stdout, "GOT REQUEST!")
	fmt.Fprint(w, "passing")
}

func main() {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	port := flag.String("port", "8087", "port to listen on")
	flag.Parse()

	env := os.Getenv("ENV_TEST")
	fmt.Fprintf(os.Stdout, "ENV: %s", env)

	http.HandleFunc("/", handler)
	fmt.Printf("Server is running on port %s\n", *port)
	go func() {
		err := http.ListenAndServe(":"+*port, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting server: %s", err)
			os.Exit(1)
		}
	}()

	<-exit
}
