package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Fprint(os.Stdout, "starting function")
	time.Sleep(750 * time.Millisecond)
	fmt.Fprint(os.Stdout, "ending function")
}
