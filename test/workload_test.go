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

	binPath, err := buildTestBinary(t, "./testdata/direct_start/main.go", workingDir)
	if err != nil {
		t.Fatal(err)
	}

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsServer(t, workingDir)
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

		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath, "--node-id", pub, "--node-xkey-pub", xkey_pub, fmt.Sprintf("--argv=-port=%d", port), fmt.Sprintf("--env=ENV_TEST=%s", "derp"))
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

func TestDirectStartFunction(t *testing.T) {
	workingDir := t.TempDir()

	binPath, err := buildTestBinary(t, "./testdata/function/main.go", workingDir)
	if err != nil {
		t.Fatal(err)
	}

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsServer(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	cmd, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	cmd.SysProcAttr = sysProcAttr()
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

	passed := false
	triggerLogs := new(bytes.Buffer)
	go func() {
		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath, "--runtype", "function", "--trigger", "test")
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

		time.Sleep(500 * time.Millisecond)

		if len(cmdstderr.Bytes()) > 0 {
			t.Error("stderr:", cmdstderr.String())
			return
		}

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
		if len(match) != 2 {
			t.Error("tester workload failed to start: ", cmdstdout.String())
			return
		}

		time.Sleep(500 * time.Millisecond)
		cmd = exec.Command(nexCli, "workload", "info", "-s", s.ClientURL(), match[1], "--json")
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		err = cmd.Run()
		if err != nil {
			t.Log(cmdstderr.String())
			t.Error(err)
			return
		}

		time.Sleep(500 * time.Millisecond)
		var resp gen.WorkloadPingResponseJson
		err = json.Unmarshal(cmdstdout.Bytes(), &resp)
		if err != nil {
			t.Error(err)
			return
		}

		if resp.WorkloadSummary.WorkloadState != models.WorkloadStateWarm {
			t.Logf("ERROR: workload state: %s; expected %s", resp.WorkloadSummary.WorkloadState, models.WorkloadStateWarm)
			t.Error(err)
			return
		}

		nc, err := nats.Connect(s.ClientURL())
		if err != nil {
			t.Error(err)
			return
		}
		defer nc.Close()

		workloadId := match[1]
		sub, err := nc.Subscribe("$NEX.logs.system."+workloadId+".stdout", func(msg *nats.Msg) {
			triggerLogs.Write(msg.Data)
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = sub.Drain()
		}()

		err = nc.Publish("test", []byte("test data 123"))
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(time.Second)

		cmd = exec.Command(nexCli, "workload", "info", "-s", s.ClientURL(), match[1], "--json")
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		err = cmd.Run()
		if err != nil {
			t.Log(cmdstderr.String())
			t.Error(err)
			return
		}

		time.Sleep(500 * time.Millisecond)
		err = json.Unmarshal(cmdstdout.Bytes(), &resp)
		if err != nil {
			t.Error(err)
			return
		}

		dur, err := time.ParseDuration(resp.WorkloadSummary.Runtime)
		if err != nil {
			t.Error(err)
			return
		}

		if dur > 500*time.Millisecond && dur < 1*time.Second {
			passed = true
		} else {
			t.Log("Job runtime: ", resp.WorkloadSummary.Runtime)
		}

		// TODO: stop workload
		cancel()
	}()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("expected workload to run for 500ms - 1s")
	}

	if !bytes.Contains(triggerLogs.Bytes(), []byte("test data 123")) {
		t.Error("expected 'test data 123' to be consumed by workload")
	}
}

func TestDirectStop(t *testing.T) {
	workingDir := t.TempDir()

	binPath, err := buildTestBinary(t, "./testdata/forever/main.go", workingDir)
	if err != nil {
		t.Fatal(err)
	}

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsServer(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	nex1, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex1.SysProcAttr = sysProcAttr()
	// nex1.Stdout = os.Stdout
	// nex1.Stderr = os.Stderr

	nex2, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node2", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex2.SysProcAttr = sysProcAttr()
	// nex2.Stdout = os.Stdout
	// nex2.Stderr = os.Stderr

	err = nex1.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = nex2.Start()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		err = stopProcess(nex1.Process)
		if err != nil {
			t.Error(err)
		}
		err = stopProcess(nex2.Process)
		if err != nil {
			t.Error(err)
		}
	}()

	passed := false
	go func() {
		cmd := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath)
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

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(cmdstdout.String()))
		if len(match) != 2 {
			t.Error("tester workload failed to start: ", cmdstdout.String())
			return
		}

		time.Sleep(500 * time.Millisecond)
		workloadID := match[1]

		cmd = exec.Command(nexCli, "workload", "stop", "-s", s.ClientURL(), workloadID)
		cmdstdout = new(bytes.Buffer)
		cmdstderr = new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		err = cmd.Run()
		if err != nil {
			t.Log(cmdstderr.String())
			t.Error(err)
			return
		}

		if bytes.Contains(cmdstdout.Bytes(), []byte("Workload "+workloadID+" stopped")) {
			passed = true
		} else {
			t.Log("cmdstdout:", cmdstdout.String())
			if len(cmdstderr.Bytes()) > 0 {
				t.Error("stderr:", cmdstderr.String())
			}
		}

		cancel()
	}()

	err = nex1.Wait()
	if err != nil {
		t.Fatal(err)
	}
	err = nex2.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("expected workload to stop")
	}
}

func TestDirectStopNoWorkload(t *testing.T) {
	workingDir := t.TempDir()

	nexCli, err := buildNexCli(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := startNatsServer(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	nex1, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex1.SysProcAttr = sysProcAttr()
	// nex1.Stdout = os.Stdout
	// nex1.Stderr = os.Stderr

	err = nex1.Start()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() {
		<-ctx.Done()
		err = stopProcess(nex1.Process)
		if err != nil {
			t.Error(err)
		}
	}()

	passed := false
	go func() {
		cmd := exec.Command(nexCli, "workload", "stop", "-s", s.ClientURL(), "fakeid")
		cmdstdout := new(bytes.Buffer)
		cmdstderr := new(bytes.Buffer)
		cmd.Stdout = cmdstdout
		cmd.Stderr = cmdstderr
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			t.Log(cmdstderr.String())
			t.Error(err)
			return
		}

		if bytes.Contains(cmdstdout.Bytes(), []byte("Workload not found")) {
			passed = true
		} else {
			t.Log("cmdstdout:", cmdstdout.String())
			if len(cmdstderr.Bytes()) > 0 {
				t.Error("stderr:", cmdstderr.String())
			}
		}

		cancel()
	}()

	err = nex1.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("failed to properly detect no workloads")
	}
}
