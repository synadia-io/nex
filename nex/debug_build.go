//go:build debug

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/trace"
	"time"

	_ "net/http/pprof"
)

var (
	node_pprof_port   *int
	node_enable_trace *bool
	node_trace_file   *string
)

func setDebuggerCommands() {
	node_pprof_port = nodeUp.Flag("pprof_port", "pprof server port").Default("6060").Int()
	node_enable_trace = nodeUp.Flag("pprof_trace", "Enable profile tracing").Default("false").UnNegatableBool()
	node_trace_file = nodeUp.Flag("pprof_trace_file", "pprof trace file location").Default(fmt.Sprintf("trace-%d.out", time.Now().Unix())).String()
}

func initDebug(logger *slog.Logger) (func() error, error) {
	var fp *os.File
	var err error

	if *node_enable_trace {
		fp, err = os.Create(*node_trace_file)
		if err != nil {
			logger.Error("Error creating trace file", slog.Any("err", err))
			return nil, err
		}

		err = trace.Start(fp)
		if err != nil {
			logger.Error("Error starting trace", slog.Any("err", err))
			return nil, err
		}
	}

	go func() {
		fmt.Println(http.ListenAndServe(fmt.Sprintf(":%d", *node_pprof_port), nil))
	}()

	logger.Info("******************* DEBUG BUILD *******************")
	logger.Info(fmt.Sprintf("pprof server started at :%d", *node_pprof_port))
	if *node_enable_trace {
		logger.Info(fmt.Sprintf("trace output at: %s", node_trace_file))
	}
	logger.Info("***************************************************")

	return func() error {
		logger.Info("Stopping trace", slog.String("file", *node_trace_file))
		trace.Stop()
		return fp.Close()
	}, nil
}
