package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Fprint(os.Stdout, "starting function")
	fmt.Fprint(os.Stdout, os.Getenv("NEX_TRIGGER_DATA"))
	time.Sleep(750 * time.Millisecond)
	fmt.Fprint(os.Stdout, "ending function")
}
