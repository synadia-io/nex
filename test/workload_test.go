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

func TestDirectStart(t *testing.T) {
	workingDir := t.TempDir()

	binPath, err := buildDirectStartBinary(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsSever(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	kp, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal(err)
	}

	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}

	cmd, err := startNexNodeCmd(t, workingDir, string(seed), s.ClientURL(), "node", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = cmd.Process.Signal(syscall.SIGINT)
	}()

	go func() {
		pub, err := kp.PublicKey()
		if err != nil {
			t.Error(err)
			return
		}

		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "--uri", "file://"+binPath, "--node-id", pub)
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		err = cmd.Run()
		if err != nil {
			t.Error(err)
			return
		}

		if len(cmdstderr.Bytes()) > 0 {
			t.Error("stderr:", cmdstderr.String())
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

		time.Sleep(500 * time.Millisecond)
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

		cancel()
	}()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

}
