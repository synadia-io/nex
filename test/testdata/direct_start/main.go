package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(os.Stdout, "GOT REQUEST!")
	fmt.Fprint(w, "passing")
}

func main() {
	port := flag.String("port", "8087", "port to listen on")
	flag.Parse()

	http.HandleFunc("/", handler)
	fmt.Printf("Server is running on port %s\n", *port)
	_ = http.ListenAndServe(":"+*port, nil)
}
