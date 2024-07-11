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

func initDebug(logger *slog.Logger) (func() error, error) {
	d, err := os.Getwd()
	if err != nil {
		logger.Error("Failed to determine CWD", slog.Any("err", err))
		return nil, err
	}
	traceFileName := fmt.Sprintf("trace-%d.out", time.Now().Unix())
	tracePath := filepath.Join(d, traceFileName)

	go func() {
		fmt.Println(http.ListenAndServe(":6060", nil))
	}()

	fp, err := os.Create(tracePath)
	if err != nil {
		logger.Error("Error creating trace file", slog.Any("err", err))
		return nil, err
	}

	err = trace.Start(fp)
	if err != nil {
		logger.Error("Error starting trace", slog.Any("err", err))
		return nil, err
	}

	logger.Info("******************* DEBUG BUILD *******************")
	logger.Info("pprof server started at :6060")
	logger.Info("trace output at: " + tracePath)
	logger.Info("***************************************************")

	return func() error {
		logger.Info("Stopping trace", slog.String("file", tracePath))
		trace.Stop()
		return fp.Close()
	}, nil
}
