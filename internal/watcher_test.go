package internal

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-labs/nex/models"
)

func TestNewAgentWatcher(t *testing.T) {
	at := NewAgentWatcher(context.TODO(), slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	be.Nonzero(t, at)
	be.Equal(t, 1, at.resetLimit)
}

func TestNewAgent(t *testing.T) {
	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	at := NewAgentWatcher(context.TODO(), logger, 1)

	uri, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	regs := models.NewRegistrationList(logger)
	ap := &AgentProcess{
		Config: &models.Agent{
			Uri:  uri,
			Argv: []string{"1"},
		},
		Id:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	regCreds := &models.NatsConnectionData{}

	at.New(regs, ap, regCreds)

	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="starting local agent" agent=sleep restart_count=0 reset_limit=1`))
	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="starting local agent" agent=sleep restart_count=1 reset_limit=1`))
	be.Equal(t, 2, strings.Count(stdout.String(), `level=WARN msg="Process unexpectedly exited with state" state="exit status 0"`))
}

func TestNewAgentBadCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	at := NewAgentWatcher(context.TODO(), logger, 1)

	regs := &models.Regs{}
	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: "foobar",
		},
		Id:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	regCreds := &models.NatsConnectionData{}

	at.New(regs, ap, regCreds)
	be.True(t, strings.Contains(stdout.String(), `level=ERROR msg="provide path is not a binary file" agent_uri=foobar err="stat foobar: no such file or directory"`))
}

func TestNewAgentBadBinary(t *testing.T) {
	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	at := NewAgentWatcher(context.TODO(), logger, 1)

	fakeBinary, err := os.CreateTemp(t.TempDir(), "fakebin*")
	be.NilErr(t, err)
	err = os.Chmod(fakeBinary.Name(), 0755)
	be.NilErr(t, err)
	fakeBinary.Close()

	regs := models.NewRegistrationList(logger)
	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: fakeBinary.Name(),
		},
		Id:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	regCreds := &models.NatsConnectionData{}

	at.New(regs, ap, regCreds)

	be.Equal(t, 2, strings.Count(stdout.String(), `level=ERROR msg="failed to start local agent"`))
}

func TestNewAgentBadCommandArgs(t *testing.T) {
	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	at := NewAgentWatcher(context.TODO(), logger, 1)

	uri, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	regs := models.NewRegistrationList(logger)
	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: uri,
		},
		Id:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	regCreds := &models.NatsConnectionData{}

	at.New(regs, ap, regCreds)

	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="starting local agent" agent=sleep restart_count=0 reset_limit=1`))
	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="starting local agent" agent=sleep restart_count=1 reset_limit=1`))

	// TODO: This adds some flakiness to the test, investigate later
	// be.Equal(t, 2, strings.Count(stdout.String(), `level=ERROR`)) // error message is different for linux/osx so the CI breaks if we include any more context
	// be.Equal(t, 2, strings.Count(stdout.String(), `level=WARN msg="Process unexpectedly exited with state" state="exit status 1"`))
}
