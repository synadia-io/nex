package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	gen "github.com/synadia-io/nex/api/go"
	"github.com/synadia-io/nex/models"
)

func TestAuctionDeploy(t *testing.T) {
	workingDir := t.TempDir()

	binPath := BuildTestBinary(t, "./testdata/direct_start/main.go", workingDir)

	nexCli := buildNexCli(t, workingDir)

	s := StartNatsServer(t, workingDir)
	defer s.Shutdown()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	nex1 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus")
	nex1.Stdout = stdout
	nex1.Stderr = stderr
	nex1.SysProcAttr = sysProcAttr()

	nex2 := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node2", "nexus")
	nex2.SysProcAttr = sysProcAttr()

	err := nex1.Start()
	be.NilErr(t, err)
	err = nex2.Start()
	be.NilErr(t, err)

	go func() {
		time.Sleep(500 * time.Millisecond)

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		// find unused port
		listener, err := net.Listen("tcp", ":0")
		be.NilErr(t, err)

		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		auctionDeploy := exec.Command(nexCli, "workload", "run", "-s", s.ClientURL(), "--name", "tester", fmt.Sprintf("--node-tags=%s=%s", models.TagNodeName, "node1"), "file://"+binPath, fmt.Sprintf("--argv=-port=%d", port), "--env=ENV_TEST=nexenvset")
		auctionDeploy.Stdout = stdout
		auctionDeploy.Stderr = stderr
		err = auctionDeploy.Run()
		be.NilErr(t, err)

		stdout = new(bytes.Buffer)
		stderr = new(bytes.Buffer)
		nodels := exec.Command(nexCli, "node", "ls", "-s", s.ClientURL(), "--json")
		nodels.Stdout = stdout
		nodels.Stderr = stderr
		err = nodels.Run()
		be.NilErr(t, err)

		lsout := []*gen.NodePingResponseJson{}
		err = json.Unmarshal(stdout.Bytes(), &lsout)
		be.NilErr(t, err)

		for _, n := range lsout {
			switch n.Tags.Tags[models.TagNodeName] {
			case "node1":
				be.Equal(t, 1, n.RunningAgents.Status["direct-start"])
			case "node2":
				be.Equal(t, 0, n.RunningAgents.Status["direct-start"])
			default:
				t.Log(stdout.String())
				t.Error("this should never happen")
			}
		}

		err = stopProcess(nex1.Process)
		be.NilErr(t, err)
		err = stopProcess(nex2.Process)
		be.NilErr(t, err)
	}()

	err = nex1.Wait()
	be.NilErr(t, err)
	err = nex2.Wait()
	be.NilErr(t, err)

	be.In(t, "ENV: nexenvset", stdout.Bytes())
}
