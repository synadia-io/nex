package test

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
)

func buildTestBinary(t testing.TB, binMain string, workingDir string) (string, error) {
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

func TestDirectStartService(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/direct_start/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	seed, err := kp.Seed()
	be.NilErr(t, err)

	xkey, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	xSeed, err := xkey.Seed()
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := startNexNodeCmd(t, workingDir, string(seed), string(xSeed), s.ClientURL(), "node", "nexus")
	cmd.SysProcAttr = sysProcAttr()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	be.NilErr(t, cmd.Start())
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, stopProcess(cmd.Process))
	}()

	go func() {
		pub, err := kp.PublicKey()
		be.NilErr(t, err)

		xkey_pub, err := xkey.PublicKey()
		be.NilErr(t, err)

		// find unused port
		listener, err := net.Listen("tcp", ":0")
		be.NilErr(t, err)
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath, "--node-id", pub, "--node-xkey-pub", xkey_pub, fmt.Sprintf("--argv=-port=%d", port), fmt.Sprintf("--env=ENV_TEST=%s", "derp"))
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		be.Equal(t, 0, len(cmdstderr.Bytes()))

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started on node (?P<node>[A-Z0-9]+)$`)
		match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
		be.Equal(t, 3, len(match))
		be.Equal(t, pub, match[2])

		time.Sleep(500 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
		be.NilErr(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		be.NilErr(t, err)

		be.True(t, strings.Contains(string(body), "passing"))

		cancel()
	}()

	be.NilErr(t, cmd.Wait())

	// This ensures that the environment was decrypted and injected into workload correctly
	be.In(t, "ENV: derp", stdout.String())
}

func TestDirectStartFunction(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/function/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	cmd := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node", "nexus")
	cmd.SysProcAttr = sysProcAttr()

	be.NilErr(t, cmd.Start())
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, stopProcess(cmd.Process))
	}()

	passed := false
	triggerLogs := new(bytes.Buffer)
	go func() {
		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath, "--runtype", "function", "--trigger", "test")
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())
		time.Sleep(500 * time.Millisecond)

		be.Equal(t, 0, len(cmdstderr.Bytes()))

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
		be.Equal(t, 2, len(match))
		time.Sleep(500 * time.Millisecond)

		cmd = exec.Command(nexCli, "workload", "info", "-s", s.ClientURL(), match[1], "--json")
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		time.Sleep(500 * time.Millisecond)
		var resp gen.WorkloadPingResponseJson
		be.NilErr(t, json.Unmarshal(cmdstdout.Bytes(), &resp))

		be.Equal(t, models.WorkloadStateWarm, resp.WorkloadSummary.WorkloadState)

		nc, err := nats.Connect(s.ClientURL())
		be.NilErr(t, err)
		defer nc.Close()

		workloadId := match[1]
		sub, err := nc.Subscribe("$NEX.logs.system."+workloadId+".stdout", func(msg *nats.Msg) {
			triggerLogs.Write(msg.Data)
		})
		be.NilErr(t, err)
		defer func() {
			_ = sub.Drain()
		}()

		err = nc.Publish("test", []byte("test data 123"))
		be.NilErr(t, err)
		time.Sleep(time.Second)

		cmd = exec.Command(nexCli, "workload", "info", "-s", s.ClientURL(), match[1], "--json")
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		time.Sleep(500 * time.Millisecond)
		be.NilErr(t, json.Unmarshal(cmdstdout.Bytes(), &resp))

		dur, err := time.ParseDuration(resp.WorkloadSummary.Runtime)
		be.NilErr(t, err)

		if dur > 500*time.Millisecond && dur < 1*time.Second {
			passed = true
		} else {
			t.Log("Job runtime: ", resp.WorkloadSummary.Runtime)
		}

		cancel()
	}()

	be.NilErr(t, cmd.Wait())
	be.True(t, passed)
	be.In(t, "test data 123", triggerLogs.String())
}

func TestDirectStop(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/forever/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	nex1 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	nex1.SysProcAttr = sysProcAttr()

	nex2 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node2", "nexus")
	nex2.SysProcAttr = sysProcAttr()

	be.NilErr(t, nex1.Start())
	be.NilErr(t, nex2.Start())
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, stopProcess(nex1.Process))
		be.NilErr(t, stopProcess(nex2.Process))
	}()

	go func() {
		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath)
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		be.Equal(t, 0, len(cmdstderr.Bytes()))

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
		be.Equal(t, 2, len(match))

		time.Sleep(500 * time.Millisecond)
		workloadID := match[1]

		cmd = exec.Command(nexCli, "workload", "stop", "-s", s.ClientURL(), workloadID)
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		be.In(t, "Workload "+workloadID+" stopped", cmdstdout.String())
		cancel()
	}()

	be.NilErr(t, nex1.Wait())
	be.NilErr(t, nex2.Wait())
}

func TestDirectStopNoWorkload(t *testing.T) {
	workingDir := t.TempDir()

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	nex1 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	nex1.SysProcAttr = sysProcAttr()

	be.NilErr(t, nex1.Start())
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, stopProcess(nex1.Process))
	}()

	go func() {
		cmd := exec.Command(nexCli, "workload", "stop", "-s", s.ClientURL(), "fakeid")
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		be.NilErr(t, cmd.Run())

		be.In(t, "Workload not found", cmdstdout.String())
		cancel()
	}()

	be.NilErr(t, nex1.Wait())
}
