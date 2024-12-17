//go:build debug

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
)

func pprof(logger *slog.Logger) {
	logger.Warn("******************* DEBUG BUILD *******************")
	logger.Warn("pprof server started at :6060")
	logger.Warn("***************************************************")
	fmt.Println(http.ListenAndServe(":6060", nil))
}
