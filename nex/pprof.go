//go:build debug

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		fmt.Println("******************* DEBUG BUILD *******************")
		fmt.Println("pprof server started at :6060")
		fmt.Println("***************************************************")
		fmt.Println(http.ListenAndServe(":6060", nil))
	}()
}
