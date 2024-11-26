package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
)

func TestCopyWorkload(t *testing.T) {
	workingDir := t.TempDir()

	binPath, err := buildTestBinary(t, "./testdata/nats_micro/main.go", workingDir)
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

	nex1, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex1.SysProcAttr = sysProcAttr()

	n2KP, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal(err)
	}

	n2KPSeed, err := n2KP.Seed()
	if err != nil {
		t.Fatal(err)
	}

	n2KPPub, err := n2KP.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	nex2, err := startNexNodeCmd(t, workingDir, string(n2KPSeed), "", s.ClientURL(), "node2", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex2.SysProcAttr = sysProcAttr()

	err = nex1.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = nex2.Start()
	if err != nil {
		t.Fatal(err)
	}

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)

		origStdOut := new(bytes.Buffer)
		origDeploy := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node1"), "--uri", "file://"+binPath, fmt.Sprintf("--env=NATS_URL=%s", s.ClientURL()))
		origDeploy.Stdout = origStdOut
		err = origDeploy.Run()
		if err != nil {
			t.Error(err)
			return
		}

		time.Sleep(500 * time.Millisecond)
		msg, err := nc.Request("health", nil, time.Second)
		if err != nil {
			t.Error(err)
			return
		}

		if string(msg.Data) != "ok" {
			t.Errorf("expected ok, got %s", string(msg.Data))
		}

		re := regexp.MustCompile(`^Workload tester \[(?P<workload>[A-Za-z0-9]+)\] started$`)
		match := re.FindStringSubmatch(strings.TrimSpace(origStdOut.String()))
		if len(match) != 2 {
			t.Error("tester workload failed to start: ", origStdOut.String())
			return
		}
		origWorkloadId := match[1]

		copyStdOut := new(bytes.Buffer)
		copyDeploy := exec.Command(nexCli, "workload", "copy", "-s", s.ClientURL(), origWorkloadId, fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node2"))
		copyDeploy.Stdout = copyStdOut
		err = copyDeploy.Run()
		if err != nil {
			t.Error(err)
			return
		}

		time.Sleep(500 * time.Millisecond)
		node2InfoStdOut := new(bytes.Buffer)
		node2InfoStdErr := new(bytes.Buffer)
		node2Info := exec.Command(nexCli, "node", "info", "-s", s.ClientURL(), "--json", n2KPPub)
		node2Info.Stdout = node2InfoStdOut
		node2Info.Stderr = node2InfoStdErr
		err = node2Info.Run()
		if err != nil {
			t.Log("stdout: ", node2InfoStdOut.String())
			t.Log("stderr: ", node2InfoStdErr.String())
			t.Error(err)
			return
		}

		resp := new(gen.NodeInfoResponseJson)
		err = json.Unmarshal(node2InfoStdOut.Bytes(), resp)
		if err != nil {
			t.Error(err)
			return
		}

		if len(resp.WorkloadSummaries) != 1 {
			t.Error("expected 1 workload, got ", len(resp.WorkloadSummaries))
			return
		}

		err = stopProcess(nex1.Process)
		if err != nil {
			t.Error(err)
		}
		err = stopProcess(nex2.Process)
		if err != nil {
			t.Error(err)
		}
	}()

	err = nex1.Wait()
	if err != nil {
		t.Fatal(err)
	}
	err = nex2.Wait()
	if err != nil {
		t.Fatal(err)
	}

}
