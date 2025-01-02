package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
)

func TestCopyWorkload(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/nats_micro/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	nex1 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	nex1.SysProcAttr = sysProcAttr()

	n2KP, err := nkeys.CreateServer()
	be.NilErr(t, err)

	n2KPSeed, err := n2KP.Seed()
	be.NilErr(t, err)

	n2KPPub, err := n2KP.PublicKey()
	be.NilErr(t, err)

	nex2 := startNexNodeCmd(t, workingDir, string(n2KPSeed), "", s.ClientURL(), "node2", "nexus")
	nex2.SysProcAttr = sysProcAttr()

	be.NilErr(t, nex1.Start())
	be.NilErr(t, nex2.Start())

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	go func() {
		time.Sleep(500 * time.Millisecond)

		origStdOut := new(bytes.Buffer)
		origDeploy := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node1"), "file://"+binPath, fmt.Sprintf("--env=NATS_URL=%s", s.ClientURL()))
		origDeploy.Stdout = origStdOut
		err = origDeploy.Run()
		be.NilErr(t, err)

		time.Sleep(500 * time.Millisecond)
		msg, err := nc.Request("health", nil, time.Second)
		be.NilErr(t, err)

		be.Equal(t, "ok", string(msg.Data))

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(origStdOut.String()))
		be.Equal(t, 2, len(match))
		origWorkloadId := match[1]

		copyStdOut := new(bytes.Buffer)
		copyDeploy := exec.Command(nexCli, "workload", "copy", "-s", s.ClientURL(), origWorkloadId, fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node2"))
		copyDeploy.Stdout = copyStdOut
		be.NilErr(t, copyDeploy.Run())

		time.Sleep(500 * time.Millisecond)
		node2InfoStdOut := new(bytes.Buffer)
		node2InfoStdErr := new(bytes.Buffer)
		node2Info := exec.Command(nexCli, "node", "info", "-s", s.ClientURL(), "--json", n2KPPub)
		node2Info.Stdout = node2InfoStdOut
		node2Info.Stderr = node2InfoStdErr
		be.NilErr(t, node2Info.Run())

		resp := new(gen.NodeInfoResponseJson)
		be.NilErr(t, json.Unmarshal(node2InfoStdOut.Bytes(), resp))

		be.Equal(t, 1, len(resp.WorkloadSummaries))

		be.NilErr(t, stopProcess(nex1.Process))
		be.NilErr(t, stopProcess(nex2.Process))
	}()

	be.NilErr(t, nex1.Wait())
	be.NilErr(t, nex2.Wait())
}

func TestMultipleCopyWorkload(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/forever/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	n1KP, err := nkeys.CreateServer()
	be.NilErr(t, err)

	n1KPSeed, err := n1KP.Seed()
	be.NilErr(t, err)

	n1KPPub, err := n1KP.PublicKey()
	be.NilErr(t, err)

	nex1 := startNexNodeCmd(t, workingDir, string(n1KPSeed), "", s.ClientURL(), "node1", "nexus")
	nex1.SysProcAttr = sysProcAttr()
	be.NilErr(t, nex1.Start())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	go func() {
		<-ctx.Done()
		be.NilErr(t, stopProcess(nex1.Process))
	}()

	passed := false
	go func() {
		time.Sleep(500 * time.Millisecond)

		origStdOut := new(bytes.Buffer)
		origDeploy := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", "file://"+binPath)
		origDeploy.Stdout = origStdOut
		be.NilErr(t, origDeploy.Run())

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(origStdOut.String()))
		be.Equal(t, 2, len(match))
		origWorkloadId := match[1]

		copyStdOut := new(bytes.Buffer)
		copyDeploy := exec.Command(nexCli, "workload", "copy", "-s", s.ClientURL(), origWorkloadId)
		copyDeploy.Stdout = copyStdOut
		be.NilErr(t, copyDeploy.Run())

		copyStdOut = new(bytes.Buffer)
		copyDeploy = exec.Command(nexCli, "workload", "copy", "-s", s.ClientURL(), origWorkloadId)
		copyDeploy.Stdout = copyStdOut
		be.NilErr(t, copyDeploy.Run())
		time.Sleep(500 * time.Millisecond)

		node2InfoStdOut := new(bytes.Buffer)
		node2InfoStdErr := new(bytes.Buffer)
		node2Info := exec.Command(nexCli, "node", "info", "-s", s.ClientURL(), "--json", n1KPPub)
		node2Info.Stdout = node2InfoStdOut
		node2Info.Stderr = node2InfoStdErr
		be.NilErr(t, node2Info.Run())

		resp := new(gen.NodeInfoResponseJson)
		be.NilErr(t, json.Unmarshal(node2InfoStdOut.Bytes(), resp))

		be.Equal(t, 3, len(resp.WorkloadSummaries))

		passed = true
		cancel()
	}()

	be.NilErr(t, nex1.Wait())
	be.True(t, passed)
}
