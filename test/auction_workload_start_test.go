package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
)

func TestAuctionDeploy(t *testing.T) {
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

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	nex1, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	nex1.Stdout = stdout
	nex1.Stderr = stderr
	// nex1.Stdout = os.Stdout
	// nex1.Stderr = os.Stderr
	nex1.SysProcAttr = sysProcAttr()

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

	go func() {
		time.Sleep(500 * time.Millisecond)

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		// find unused port
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Error(err)
			return
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		auctionDeploy := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node1"), "file://"+binPath, fmt.Sprintf("--argv=-port=%d", port), "--env=ENV_TEST=nexenvset")
		auctionDeploy.Stdout = stdout
		auctionDeploy.Stderr = stderr
		err = auctionDeploy.Run()
		if err != nil {
			t.Error(err)
		}

		stdout = new(bytes.Buffer)
		stderr = new(bytes.Buffer)
		nodels := exec.Command(nexCli, "node", "ls", "-s", s.ClientURL(), "--json")
		nodels.Stdout = stdout
		nodels.Stderr = stderr
		err = nodels.Run()
		if err != nil {
			t.Error(err)
		}

		lsout := []*gen.NodePingResponseJson{}
		err = json.Unmarshal(stdout.Bytes(), &lsout)
		if err != nil {
			t.Log(stdout.String())
			t.Log(stderr.String())
			t.Error(err)
		}

		for _, n := range lsout {
			switch n.Tags.Tags[models.TagNodeName] {
			case "node1":
				if n.RunningAgents.Status["direct-start"] != 1 {
					t.Error("node1 does not have expected workload running")
				}
			case "node2":
				if n.RunningAgents.Status["direct-start"] != 0 {
					t.Error("node2 has unexpected workloads running")
				}
			default:
				t.Log(stdout.String())
				t.Error("this should never happen")
			}
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

	if !bytes.Contains(stdout.Bytes(), []byte("ENV: nexenvset")) {
		t.Error("Expected ENV Data missing | stdout:", stdout.String())
	}
}
