package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func buildDirectStartBinary(t testing.TB, binMain string, workingDir string) (string, error) {
	t.Helper()
	binName := func() string {
		if runtime.GOOS == "windows" {
			return "test.exe"
		}
		return "test"
	}

	if _, err := os.Stat(filepath.Join(workingDir, binName())); err == nil {
		return filepath.Join(workingDir, binName()), nil
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(workingDir, binName()), binMain)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filepath.Join(workingDir, binName())); err != nil {
		return "", err
	}

	return filepath.Join(workingDir, binName()), nil
}

func TestDirectStart(t *testing.T) {
	workingDir := t.TempDir()

	binPath, err := buildDirectStartBinary(t, "./testdata/direct_start/main.go", workingDir)
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

	xkey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Error(err)
		return
	}

	xSeed, err := xkey.Seed()
	if err != nil {
		t.Error(err)
		return
	}

	cmd, err := startNexNodeCmd(t, workingDir, string(seed), string(xSeed), s.ClientURL(), "node", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	cmd.SysProcAttr = sysProcAttr()
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
		err = stopProcess(cmd.Process)
		if err != nil {
			t.Error(err)
		}
	}()

	go func() {
		pub, err := kp.PublicKey()
		if err != nil {
			t.Error(err)
			return
		}

		xkey_pub, err := xkey.PublicKey()
		if err != nil {
			t.Error(err)
			return
		}

		// find unused port
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Error(err)
			return
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "--uri", "file://"+binPath, "--node-id", pub, "--node-xkey-pub", xkey_pub, fmt.Sprintf("--argv=-port=%d", port), fmt.Sprintf("--env=ENV_TEST=%s", "derp"))
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		err = cmd.Run()
		if err != nil {
			t.Log(cmdstderr.String())
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

		time.Sleep(5000 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
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
		cancel()
	}()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	// This ensures that the environment was decrypted and injected into workload correctly
	if !bytes.Contains(stdout.Bytes(), []byte("ENV: derp")) {
		t.Error("Expected ENV Data missing | stdout:", stdout.String())
	}
}
