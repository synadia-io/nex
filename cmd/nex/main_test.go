package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/nats-io/nkeys"
)

func TestCLISimple(t *testing.T) {
	nex := NexCLI{}

	parser := kong.Must(&nex,
		kong.Vars(map[string]string{"versionOnly": "testing","defaultConfigPath": t.TempDir(), "defaultResourcePath": "."}),
		kong.Bind(&nex.Globals),
	)

	kp, err := nkeys.CreatePair(nkeys.PrefixByteServer)
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := kp.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	_, err = parser.Parse([]string{"node", "info", pubKey})
	if err != nil {
		t.Fatal(err)
	}

	if nex.Node.Info.NodeID != pubKey {
		t.Fatalf("Expected %s, got %s", pubKey, nex.Node.Info.NodeID)
	}
}

func TestCLIWithConfig(t *testing.T) {
	config := `{
    "namespace": "derp",
    "workload_types": [
      {
        "name": "CONFIGWORKLOAD",
        "agenturi": "nats://uri/config",
        "argv": [],
        "env": {
          "FOO": "BAR"
        }
      },
      {
        "name": "CONFIGWORKLOAD_TWO",
        "agenturi": "nats://uri/config2",
        "argv": ["--arg", "foo"],
        "env": {}
      }
    ]
  }`

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "nex.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(config)
	defer f.Close()

<<<<<<< HEAD
	nex := NexCLI{}
	parser := kong.Must(&nex,
		kong.Vars(map[string]string{"versionOnly": "testing", "defaultResourcePath": "."}),
		kong.Configuration(kong.JSON, f.Name()),
		kong.Bind(&nex.Globals),
	)
=======
	nex := new(NexCLI)
	parser := kong.Must(nex, kong.Vars(map[string]string{"versionOnly": "testing", "defaultConfigPath": t.TempDir(), "defaultResourcePath": "."}), kong.Configuration(kong.JSON, f.Name()))
>>>>>>> 4ca7457 (missing vars)
	parser.LoadConfig(f.Name())

	_, err = parser.Parse([]string{"node", "up", "--config", f.Name()})
	if err != nil {
		t.Fatal(err)
	}

	if string(nex.Globals.Config) != f.Name() {
		t.Fatal("Expected config to be loaded")
	}

	if len(nex.Node.Up.WorkloadTypes) != 2 {
		t.Fatalf("Expected 2 workload types, got %d", len(nex.Node.Up.WorkloadTypes))
	}
}
