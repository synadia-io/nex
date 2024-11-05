package test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

func buildNexCli(t testing.TB, workingDir string) (string, error) {
	t.Helper()
	err := os.Chdir("../cmd/nex")
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", workingDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return "", err
	}

	err = cmd.Wait()
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, "nex"), nil
}

func startNatsSever(t testing.TB, workingDir string) (*server.Server, error) {
	t.Helper()

	s := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workingDir,
	})

	s.Start()

	go s.WaitForShutdown()

	return s, nil
}

func startNextNodeCmd(t testing.TB, workingDir string) (*exec.Cmd, *server.Server, error) {
	t.Helper()
	s, err := startNatsSever(t, workingDir)
	if err != nil {
		return nil, nil, err
	}

	cli, err := buildNexCli(t, workingDir)
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.Command(cli, "node", "up", "--logger.level", "debug", "--logger.short", "-s", s.ClientURL())

	return cmd, s, nil
}

func TestStartNode(t *testing.T) {
	workingDir := t.TempDir()
	cmd, s, err := startNextNodeCmd(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	passed := false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		ticker := time.NewTicker(time.Millisecond * 500)
		select {
		case <-ticker.C:
			if bytes.Contains(stdout.Bytes(), []byte("NATS execution engine awaiting commands")) {
				passed = true
				_ = cmd.Process.Signal(os.Interrupt)
			}
		case <-ctx.Done():
			_ = cmd.Process.Signal(os.Interrupt)
		}
	}()

	err = cmd.Wait()
	if err != nil {
		t.Log(stdout.String())
		t.Log(stderr.String())
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("Nex Node did not start")
	}
}
