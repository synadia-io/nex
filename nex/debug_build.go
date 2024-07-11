//go:build debug

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/trace"
	"time"

	_ "net/http/pprof"
)

func initDebug(logger *slog.Logger) func() error {
	d, _ := os.Getwd()
	traceFileName := fmt.Sprintf("trace-%d.out", time.Now().Unix())
	tracePath := filepath.Join(d, traceFileName)

	go func() {
		fmt.Println(http.ListenAndServe(":6060", nil))
	}()

	fp, err := os.Create(tracePath)
	if err != nil {
		fmt.Println("Error creating trace file: ", err)
		os.Exit(1)
	}

	err = trace.Start(fp)
	if err != nil {
		fmt.Println("Error starting trace: ", err)
		os.Exit(1)
	}

	logger.Info("******************* DEBUG BUILD *******************")
	logger.Info("pprof server started at :6060")
	logger.Info("trace output at: " + tracePath)
	logger.Info("***************************************************")

	return func() error {
		logger.Info("Stopping trace", slog.String("file", tracePath))
		trace.Stop()
		return fp.Close()
	}
}
