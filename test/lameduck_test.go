package test

import (
	"bytes"
	"os/exec"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func TestLameDuckMode(t *testing.T) {
	workingDir := t.TempDir()

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

	cmd, err := startNexNodeCmd(t, workingDir, string(seed), "", s.ClientURL(), "node", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	cmd.SysProcAttr = sysProcAttr()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	ldStdout := new(bytes.Buffer)
	go func() {
		pub, err := kp.PublicKey()
		if err != nil {
			t.Error(err)
			return
		}

		cmd := exec.Command(nexCli, "node", "lameduck", "-s", s.ClientURL(), "--node-id", pub)
		cmd.Stdout = ldStdout
		err = cmd.Run()
		if err != nil {
			t.Error(err)
			return
		}
	}()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if len(stderr.Bytes()) > 0 {
		t.Errorf("expected no output from stderr: %s", stderr.String())
	}

	if !bytes.Contains(ldStdout.Bytes(), []byte("is now in lameduck mode. Workloads will begin stopping gracefully.")) {
		t.Errorf("expected lameduck message, got: %s", ldStdout.String())
	}

	if !bytes.Contains(stdout.Bytes(), []byte("Received lame duck request")) ||
		!bytes.Contains(stdout.Bytes(), []byte("Shutting down nexnode")) {
		t.Errorf("expected node logs not present, got: %s", stdout.String())
	}
}
