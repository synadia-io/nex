package test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
)

func buildNexCli(t testing.TB, workingDir string) (string, error) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(workingDir, "nex")); err == nil {
		return filepath.Join(workingDir, "nex"), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	err = os.Chdir("../cmd/nex")
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", workingDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return "", err
	}

	err = cmd.Wait()
	if err != nil {
		return "", err
	}

	err = os.Chdir(cwd)
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, "nex"), nil
}

func startNatsServer(t testing.TB, workingDir string) (*server.Server, error) {
	t.Helper()

	s := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workingDir,
	})

	s.Start()

	go s.WaitForShutdown()

	return s, nil
}

func startNexNodeCmd(t testing.TB, workingDir, nodeSeed, xkeySeed, natsServer, name, nexus string) (*exec.Cmd, error) {
	t.Helper()

	cli, err := buildNexCli(t, workingDir)
	if err != nil {
		return nil, err
	}

	if nodeSeed == "" {
		kp, err := nkeys.CreateServer()
		if err != nil {
			return nil, err
		}
		s, err := kp.Seed()
		if err != nil {
			return nil, err
		}
		nodeSeed = string(s)
	}

	if xkeySeed == "" {
		xkp, err := nkeys.CreateCurveKeys()
		if err != nil {
			return nil, err
		}
		xSeed, err := xkp.Seed()
		if err != nil {
			return nil, err
		}
		xkeySeed = string(xSeed)
	}

	cmd := exec.Command(cli, "node", "up", "--logger.level", "debug", "--logger.short", "-s", natsServer, "--resource-directory", workingDir, "--node-name", name, "--nexus", nexus, "--node-seed", nodeSeed, "--node-xkey-seed", xkeySeed)
	return cmd, nil
}

func TestStartNode(t *testing.T) {
	workingDir := t.TempDir()
	s, err := startNatsServer(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}

	cmd, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node", "nexus")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = sysProcAttr()

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	passed := false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		ticker := time.NewTicker(time.Millisecond * 500)
		for {
			select {
			case <-ctx.Done():
				err = stopProcess(cmd.Process)
				if err != nil {
					t.Error(err)
				}
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

	err = cmd.Wait()
	if err != nil {
		t.Log(stdout.String())
		t.Log(stderr.String())
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("Nex Node did not start")
	}
}

func TestStartNexus(t *testing.T) {
	workingDir := t.TempDir()
	s, err := startNatsServer(t, workingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	nex1, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node1", "nexus3node")
	if err != nil {
		t.Fatal(err)
	}
	nex1.SysProcAttr = sysProcAttr()
	nex2, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node2", "nexus3node")
	if err != nil {
		t.Fatal(err)
	}
	nex2.SysProcAttr = sysProcAttr()
	nex3, err := startNexNodeCmd(t, workingDir, "", "", s.ClientURL(), "node3", "nexus3node")
	if err != nil {
		t.Fatal(err)
	}
	nex3.SysProcAttr = sysProcAttr()

	err = nex1.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = nex2.Start()
	if err != nil {
		t.Fatal(err)
	}
	err = nex3.Start()
	if err != nil {
		t.Fatal(err)
	}

	passed := false
	go func() {
		time.Sleep(time.Millisecond * 500)

		nexPath, err := buildNexCli(t, workingDir)
		if err != nil {
			t.Error(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		ticker := time.NewTicker(time.Second * 1)
		for {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			nodels := exec.Command(nexPath, "node", "ls", "-s", s.ClientURL(), "--json")
			nodels.Stdout = stdout
			nodels.Stderr = stderr

			select {
			case <-ctx.Done():
				err = stopProcess(nex1.Process)
				if err != nil {
					t.Error(err)
				}
				err = stopProcess(nex2.Process)
				if err != nil {
					t.Error(err)
				}
				err = stopProcess(nex3.Process)
				if err != nil {
					t.Error(err)
				}
				return
			case <-ticker.C:
				err := nodels.Run()
				if err != nil {
					t.Error(err)
					ticker.Stop()
					cancel()
				}

				if len(stderr.Bytes()) != 0 {
					t.Log("stderr:", stderr.String())
					cancel()
				}
				if len(stdout.Bytes()) == 0 {
					continue
				}

				resp := []*gen.NodePingResponseJson{}
				err = json.Unmarshal(stdout.Bytes(), &resp)
				if err != nil {
					t.Error()
					ticker.Stop()
					cancel()
				}

				if len(resp) == 3 {
					passed = true
					ticker.Stop()
					cancel()
				}
			}
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
	err = nex3.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if !passed {
		t.Fatal("Three Nex Nodes did not start")
	}
}
