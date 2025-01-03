package test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
)

func TestStartNode(t *testing.T) {
	workingDir := t.TempDir()
	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	cmd := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node", "nexus")

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = sysProcAttr()

	be.NilErr(t, cmd.Start())

	passed := false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		ticker := time.NewTicker(time.Millisecond * 500)
		for {
			select {
			case <-ctx.Done():
				be.NilErr(t, stopProcess(cmd.Process))
				return
			case <-ticker.C:
				if bytes.Contains(stdout.Bytes(), []byte("NATS execution engine awaiting commands")) {
					passed = true
					ticker.Stop()
					cancel()
				}
			}
		}
	}()

	be.NilErr(t, cmd.Wait())
	be.True(t, passed)
}

func TestStartNexus(t *testing.T) {
	workingDir := t.TempDir()
	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	nex1 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus3node")
	nex1.SysProcAttr = sysProcAttr()
	nex2 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node2", "nexus3node")
	nex2.SysProcAttr = sysProcAttr()
	nex3 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node3", "nexus3node")
	nex3.SysProcAttr = sysProcAttr()

	be.NilErr(t, nex1.Start())
	be.NilErr(t, nex2.Start())
	be.NilErr(t, nex3.Start())

	passed := false
	go func() {
		time.Sleep(time.Millisecond * 500)

		nexPath := buildNexCli(t, workingDir)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		ticker := time.NewTicker(time.Second * 1)
		defer ticker.Stop()

		for {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			nodels := exec.Command(nexPath, "node", "ls", "-s", s.ClientURL(), "--json")
			nodels.Stdout = stdout
			nodels.Stderr = stderr

			select {
			case <-ctx.Done():
				be.NilErr(t, stopProcess(nex1.Process))
				be.NilErr(t, stopProcess(nex2.Process))
				be.NilErr(t, stopProcess(nex3.Process))
				return
			case <-ticker.C:
				be.NilErr(t, nodels.Run())
				be.Equal(t, 0, len(stderr.Bytes()))

				if len(stdout.Bytes()) == 0 {
					continue
				}

				resp := []*gen.NodePingResponseJson{}
				be.NilErr(t, json.Unmarshal(stdout.Bytes(), &resp))

				if len(resp) == 3 {
					passed = true
					ticker.Stop()
					cancel()
				}
			}
		}
	}()

	be.NilErr(t, nex1.Wait())
	be.NilErr(t, nex2.Wait())
	be.NilErr(t, nex3.Wait())
	be.True(t, passed)
}
