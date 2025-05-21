package main

import (
	"fmt"
	"strings"
	"sync"
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
	defer func() {
		for s.NumClients() == 0 {
			s.Shutdown()
			return
		}
	}()

	today := time.Now().Format("2006-01-02")

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(2)

	_, err = nc.Subscribe(models.LogAPIPrefix(models.SystemNamespace)+".>", func(m *nats.Msg) {
		switch strings.Contains(m.Subject, "stderr") {
		case true:
			be.Equal(t, fmt.Sprintf("%s [ERR] error\n", today), string(m.Data))
			wg.Done()
		case false:
			be.Equal(t, fmt.Sprintf("%s [DBG] debug\n", today), string(m.Data))
			wg.Done()
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
			LogWithPid:      false,
			LogGroupOnRight: false,
		},
	}

	logger := configureLogger(globals, nc, TestServerPublicKey, false)

	logger.Debug("debug")
	logger.Error("error")

	wg.Wait()
}
