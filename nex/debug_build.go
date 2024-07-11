//go:build debug

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/trace"
	"time"

	_ "net/http/pprof"
)

func init() {
	d, _ := os.Getwd()
	traceFileName := fmt.Sprintf("trace-%d.out", time.Now().Unix())
	tracePath := filepath.Join(d, traceFileName)
	fmt.Println("******************* DEBUG BUILD *******************")
	fmt.Println("pprof server started at :6060")
	fmt.Println("trace output at: " + tracePath)
	fmt.Println("***************************************************")

	go func() {
		fmt.Println(http.ListenAndServe(":6060", nil))
	}()

	fp, err := os.Create(tracePath)
	if err != nil {
		fmt.Println("Error creating trace file: ", err)
		return
	}
	defer fp.Close()

	err = trace.Start(fp)
	if err != nil {
		fmt.Println("Error starting trace: ", err)
		return
	}
	defer trace.Stop()
}
