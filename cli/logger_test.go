package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

const (
	TestServerSeed      string = "SNAKTNRC5J7EI5BYCKQJMBACUATP7NS4D3Z2DMGUVRPIDO4AHKVNJOVF3E"
	TestServerPublicKey string = "ND6CTZ74S3J6BRNKR6Z6FWH27YMORRAQGYPNJSS7WNNZIKZIRKWLQJHR"
)

func startNatsServer(t testing.TB) *server.Server {
	t.Helper()

	s, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	be.NilErr(t, err)

	s.Start()
	return s
}

func TestConfigureLogger(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	pid := os.Getpid()
	date := time.Now().Format("2006-01-02")

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	_, err = nc.Subscribe(fmt.Sprintf("%s.>", models.LogAPIPrefix), func(m *nats.Msg) {
		switch strings.Contains(m.Subject, "stderr") {
		case true:
			stderr.Write(m.Data)
		case false:
			stdout.Write(m.Data)
		}
	})
	be.NilErr(t, err)

	globals := &Globals{
		GlobalLogger: GlobalLogger{
			Target:          []string{"nats"},
			LogLevel:        "debug",
			LogColor:        false,
			LogShortLevels:  true,
			LogTimeFormat:   "DateOnly",
			LogWithPid:      true,
			LogGroupOnRight: false,
		},
	}

	logger := configureLogger(globals, nc, TestServerPublicKey, false)

	logger.Debug("debug")
	logger.Error("error")

	time.Sleep(100 * time.Millisecond)

	be.Equal(t, stdout.String(), fmt.Sprintf("[%d] %s [DBG] debug\n", pid, date))
	be.Equal(t, stderr.String(), fmt.Sprintf("[%d] %s [ERR] error\n", pid, date))
}
