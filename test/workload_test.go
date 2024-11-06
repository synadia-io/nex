package test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func buildDirectStartBinary(t testing.TB, workingDir string) (string, error) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(workingDir, "test")); err == nil {
		return filepath.Join(workingDir, "test"), nil
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(workingDir, "test"), "./testdata/direct_start/main.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, "test"), nil
}

func runCmdOnNode(t testing.TB, ctx context.Context, workingDir, seed, natsURL string, inCmd *exec.Cmd) ([]byte, []byte, error) {
	t.Helper()

	cmd, err := startNexNodeCmd(t, workingDir, seed, natsURL, "node", "nexus")
	if err != nil {
		return nil, nil, err
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	// cmd.Stdout = stdout
	// cmd.Stderr = stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	time.Sleep(500 * time.Millisecond)

	go func() {
		<-ctx.Done()
		_ = cmd.Process.Signal(syscall.SIGINT)
	}()

	go func() {
		err := inCmd.Run()
		if err != nil {
			t.Error(err)
		}
		_ = cmd.Process.Signal(syscall.SIGINT)
	}()

	err = cmd.Wait()
	if err != nil {
		t.Log(4)
		return nil, nil, err
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}

func TestDirectStart(t *testing.T) {
	workingDir := t.TempDir()
	t.Log(workingDir)

	binPath, err := buildDirectStartBinary(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsSever(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(nexCli)

	kp, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal(err)
	}

	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}

	pub, err := kp.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "--uri", "file://"+binPath, "--node-id", pub)
	cmdstdout := new(bytes.Buffer)
	cmdstderr := new(bytes.Buffer)
	cmd.Stdout = cmdstdout
	cmd.Stderr = cmdstderr

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	_, nodeerr, err := runCmdOnNode(t, ctx, workingDir, string(seed), s.ClientURL(), cmd)
	if err != nil {
		t.Fatal(err)
	}

	if len(nodeerr) > 0 {
		t.Log("nodeerr: ", string(nodeerr))
		t.Error("nodeerr should be empty", string(nodeerr))
		return
	}

	re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started on node (?P<node>[A-Z0-9]+)$`)
	match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
	if len(match) != 3 {
		t.Error("tester workload failed to start: ", cmdstdout.String())
		return
	}

	if pub != match[2] {
		t.Error("expected node id", pub, "got", match[2])
	}

	resp, err := http.Get("http://localhost:8087")
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	if string(body) != "passing" {
		t.Error("expected passing, got", string(body))
	}

	// TODO: stop workload
}
