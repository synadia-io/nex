package test

import (
	"bytes"
	"os/exec"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
)

func TestLameDuckMode(t *testing.T) {
	workingDir := t.TempDir()

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	seed, err := kp.Seed()
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := startNexNodeCmd(t, workingDir, string(seed), "", s.ClientURL(), "node", "nexus")
	cmd.SysProcAttr = sysProcAttr()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	be.NilErr(t, cmd.Start())
	time.Sleep(500 * time.Millisecond)

	ldStdout := new(bytes.Buffer)
	go func() {
		pub, err := kp.PublicKey()
		be.NilErr(t, err)

		cmd := exec.Command(nexCli, "node", "lameduck", "-s", s.ClientURL(), "--node-id", pub)
		cmd.Stdout = ldStdout
		be.NilErr(t, cmd.Run())
	}()

	be.NilErr(t, cmd.Wait())

	be.Equal(t, 0, len(stderr.Bytes()))

	be.In(t, "is now in lameduck mode. Workloads will begin stopping gracefully.", ldStdout.String())
	be.In(t, "Received lame duck request", stdout.String())
	be.In(t, "Shutting down nexnode", stdout.String())
}
